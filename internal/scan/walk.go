package scan

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"lectern/internal/fsutil"
)

// RootSectionTitle names the implicit section that holds files placed
// directly in a course folder.
const RootSectionTitle = "Course files"

const (
	maxDepth    = 12
	maxMetaFile = 256 << 10
)

// WalkOptions tune the folder walk.
type WalkOptions struct {
	// FollowSymlinks lets links point outside the library root. Without it,
	// links are only followed when their target stays inside the root.
	FollowSymlinks bool
	Log            *slog.Logger
}

type ScannedSubtitle struct{ RelPath, Lang, Label, Format string }

type ScannedItem struct {
	RelPath   string // relative to the course folder, slash separated
	Name      string // file name with extension
	Title     string
	Kind      string
	Ext       string
	Mime      string
	Size      int64
	Mtime     time.Time
	LinkURL   string
	Subtitles []ScannedSubtitle
	Chapters  []Chapter // from a "<name>.chapters.txt" next to a video or audio file

	abs string // absolute path on disk, for probing and fingerprinting
}

type ScannedSection struct {
	RelPath string // "" for the implicit section of loose files
	Title   string
	Items   []ScannedItem
}

type ScannedCourse struct {
	RelPath     string // folder (or file) name, relative to the library root
	Title       string
	Description string
	CoverRel    string // relative to the course folder
	DirMtime    time.Time
	Sections    []ScannedSection
	Err         error // set when the folder could not be read completely

	// Strays are subtitle files that sit among videos but match none of them
	// by name (relative to the course folder). They are not lessons.
	Strays []string

	// IsFile marks a course that is a single video or audio file in the library
	// root. Its one item has the empty path (the course "folder" is the file
	// itself), and what belongs to it (subtitles, picture) is reached as
	// "../name" from there.
	IsFile bool
}

// ItemCount is the number of items over all sections.
func (c *ScannedCourse) ItemCount() int {
	n := 0
	for _, s := range c.Sections {
		n += len(s.Items)
	}
	return n
}

type ent struct {
	name  string
	abs   string
	real  string // resolved directory path, for loop detection
	isDir bool
	info  fs.FileInfo
}

type walker struct {
	ctx      context.Context
	rootReal string
	follow   bool
	log      *slog.Logger
	stack    map[string]bool
}

// Walk reads a library root and returns its courses. The layout is
//
//	library/course/section/files...
//
// Folders deeper than a section are flattened into it (their names become
// part of the item title), files directly in a course folder go to an implicit
// "Course files" section (a course without section folders is just that one
// section), and the first cover/poster/folder image is used as the course
// picture. A video or audio file placed directly in the library root is a
// course of its own with that one lesson.
func Walk(ctx context.Context, root string, opt WalkOptions) ([]ScannedCourse, error) {
	rootReal, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, fmt.Errorf("library path unavailable: %w", err)
	}
	if fi, err := os.Stat(rootReal); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("library path %q is not a directory", root)
	}
	log := opt.Log
	if log == nil {
		log = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	w := &walker{ctx: ctx, rootReal: rootReal, follow: opt.FollowSymlinks, log: log, stack: map[string]bool{rootReal: true}}
	ents, err := w.readDir(rootReal)
	if err != nil {
		return nil, fmt.Errorf("reading library: %w", err)
	}
	var courses []ScannedCourse
	var loose []ent
	for _, e := range ents {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !e.isDir {
			loose = append(loose, e)
			continue
		}
		courses = append(courses, w.course(e))
	}
	return append(courses, fileCourses(loose)...), nil
}

// fileCourses turns the video and audio files found directly in the library
// root into one-lesson courses. Files with the same name beside one (subtitles,
// a timestamp file, a picture) belong to it; any other loose file is ignored.
func fileCourses(files []ent) []ScannedCourse {
	var out []ScannedCourse
	items, _ := buildItems("", files, "")
	for _, it := range items {
		if !IsMedia(it.Kind) {
			continue
		}
		c := ScannedCourse{
			RelPath:  it.Name,
			Title:    stemOf(it.Name),
			DirMtime: it.Mtime,
			IsFile:   true,
		}
		if pic := sidePicture(files, stemOf(it.Name)); pic != "" {
			c.CoverRel = "../" + pic
		}
		for i := range it.Subtitles {
			it.Subtitles[i].RelPath = "../" + it.Subtitles[i].RelPath
		}
		it.RelPath = ""
		c.Sections = []ScannedSection{{RelPath: "", Title: RootSectionTitle, Items: []ScannedItem{it}}}
		out = append(out, c)
	}
	return out
}

// sidePicture finds the picture that goes with a media file: an image named
// like it ("Talk.jpg"), or like it plus -poster, -cover, -thumb or -thumbnail
// ("Talk-poster.png", "Talk.cover.webp").
func sidePicture(files []ent, stem string) string {
	for _, f := range files {
		if !coverExts[extOf(f.name)] {
			continue
		}
		base := stemOf(f.name)
		if len(base) < len(stem) || !strings.EqualFold(base[:len(stem)], stem) {
			continue
		}
		switch strings.ToLower(base[len(stem):]) {
		case "", "-poster", ".poster", "-cover", ".cover", "-thumb", ".thumb", "-thumbnail", ".thumbnail":
			return f.name
		}
	}
	return ""
}

func (w *walker) readDir(abs string) ([]ent, error) {
	des, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	out := make([]ent, 0, len(des))
	for _, de := range des {
		name := de.Name()
		if Ignored(name) {
			continue
		}
		p := filepath.Join(abs, name)
		e := ent{name: name, abs: p}
		switch {
		case de.Type()&fs.ModeSymlink != 0:
			real, err := filepath.EvalSymlinks(p)
			if err != nil {
				continue // dangling link
			}
			if !w.follow && !fsutil.Within(w.rootReal, real) {
				w.log.Debug("scan: skipping symlink that leaves the library", "path", p)
				continue
			}
			fi, err := os.Stat(p)
			if err != nil || (!fi.IsDir() && !fi.Mode().IsRegular()) {
				continue
			}
			e.info, e.isDir, e.real = fi, fi.IsDir(), real
		case de.IsDir():
			fi, err := de.Info()
			if err != nil {
				continue
			}
			real, err := filepath.EvalSymlinks(p)
			if err != nil {
				real = p
			}
			e.info, e.isDir, e.real = fi, true, real
		case de.Type().IsRegular():
			fi, err := de.Info()
			if err != nil {
				continue
			}
			e.info = fi
		default:
			continue // sockets, devices, pipes
		}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool { return NaturalCompare(out[i].name, out[j].name) < 0 })
	return out, nil
}

func (w *walker) course(e ent) ScannedCourse {
	c := ScannedCourse{RelPath: e.name, Title: e.name, DirMtime: e.info.ModTime().UTC().Truncate(time.Millisecond)}
	if w.stack[e.real] {
		c.Err = fmt.Errorf("symlink loop")
		return c
	}
	w.stack[e.real] = true
	defer delete(w.stack, e.real)

	ents, err := w.readDir(e.abs)
	if err != nil {
		c.Err = err
		return c
	}
	meta := w.readMeta(e.abs)
	if meta.Title != "" {
		c.Title = meta.Title
	}
	c.Description = meta.Description

	var rootFiles []ent
	var dirs []ent
	for _, x := range ents {
		switch {
		case x.isDir:
			dirs = append(dirs, x)
		case IsCoverName(x.name):
			if c.CoverRel == "" {
				c.CoverRel = x.name
			}
		default:
			rootFiles = append(rootFiles, x)
		}
	}

	items, strays := buildItems("", rootFiles, "")
	c.Strays = append(c.Strays, strays...)
	if len(items) > 0 {
		c.Sections = append(c.Sections, ScannedSection{RelPath: "", Title: RootSectionTitle, Items: items})
	}

	var secs []ScannedSection
	for _, d := range dirs {
		if err := w.ctx.Err(); err != nil {
			c.Err = err
			return c
		}
		items, strays, err := w.collect(d, d.name, "", 1)
		if err != nil {
			c.Err = fmt.Errorf("section %q: %w", d.name, err)
			return c
		}
		c.Strays = append(c.Strays, strays...)
		if len(items) == 0 {
			continue
		}
		secs = append(secs, ScannedSection{RelPath: d.name, Title: CleanTitle(d.name), Items: items})
	}
	applySectionOrder(secs, meta.Order)
	c.Sections = append(c.Sections, secs...)
	return c
}

// collect gathers the items of a section folder and everything below it.
// Files directly inside come first, then each sub-folder in natural order.
func (w *walker) collect(dir ent, rel, titlePrefix string, depth int) (items []ScannedItem, strays []string, err error) {
	if w.stack[dir.real] {
		return nil, nil, nil // symlink loop: ignore the repeat
	}
	w.stack[dir.real] = true
	defer delete(w.stack, dir.real)

	ents, err := w.readDir(dir.abs)
	if err != nil {
		return nil, nil, err
	}
	var files, dirs []ent
	for _, x := range ents {
		if x.isDir {
			dirs = append(dirs, x)
		} else {
			files = append(files, x)
		}
	}
	items, strays = buildItems(rel, files, titlePrefix)
	if depth >= maxDepth {
		return items, strays, nil
	}
	for _, d := range dirs {
		if err := w.ctx.Err(); err != nil {
			return nil, nil, err
		}
		sub, subStrays, err := w.collect(d, rel+"/"+d.name, titlePrefix+CleanTitle(d.name)+" / ", depth+1)
		if err != nil {
			return nil, nil, err
		}
		items = append(items, sub...)
		strays = append(strays, subStrays...)
	}
	return items, strays, nil
}

// buildItems turns the files of one folder into items. Subtitle files and
// timestamp files ("Talk.chapters.txt") that belong to a video or audio file
// are attached to it instead of being listed.
//
// A subtitle file is matched to a video by name (see SubtitleLang). One that
// matches none is a stray: with several videos in the folder it is left out
// and returned (as a path relative to the course folder) so the admin can fix
// its name; with a single video it is attached to it if its name says nothing
// but "english" or "subtitles"; with no video at all in the folder it is just a
// text file, as before.
func buildItems(dirRel string, files []ent, titlePrefix string) (items []ScannedItem, strays []string) {
	var subs, stamps []ent
	mk := func(f ent) ScannedItem {
		ext := extOf(f.name)
		ki := Classify(ext)
		it := ScannedItem{
			RelPath: path.Join(dirRel, f.name),
			Name:    f.name,
			Title:   titlePrefix + CleanTitle(stemOf(f.name)),
			Kind:    ki.Kind,
			Ext:     ext,
			Mime:    ki.Mime,
			Size:    f.info.Size(),
			Mtime:   f.info.ModTime().UTC().Truncate(time.Millisecond),
			abs:     f.abs,
		}
		if it.Kind == "link" {
			if u, ok := ReadLink(f.abs, ext); ok {
				it.LinkURL = u
			} else {
				it.Kind, it.Mime = "other", "application/octet-stream"
			}
		}
		return it
	}
	for _, f := range files {
		if ext := extOf(f.name); ext == "srt" || ext == "vtt" {
			subs = append(subs, f)
			continue
		}
		if _, ok := ChapterStem(f.name); ok {
			stamps = append(stamps, f)
			continue
		}
		items = append(items, mk(f))
	}
	attach := func(i int, s ent, lang string) {
		items[i].Subtitles = append(items[i].Subtitles, ScannedSubtitle{
			RelPath: path.Join(dirRel, s.name),
			Lang:    lang,
			Label:   LangLabel(lang),
			Format:  extOf(s.name),
		})
	}
	var loose []ent // subtitle files no video claims by name
	for _, s := range subs {
		sStem := stemOf(s.name)
		best, bestLen, bestLang := -1, -1, ""
		for i := range items {
			if !IsMedia(items[i].Kind) {
				continue
			}
			// The video's name without its extension ("lesson") and with it
			// ("lesson.mp4", as in "lesson.mp4.en.srt"); the longer match wins.
			for _, base := range [...]string{stemOf(items[i].Name), items[i].Name} {
				if lang, ok := SubtitleLang(base, sStem); ok && len(base) > bestLen {
					best, bestLen, bestLang = i, len(base), lang
				}
			}
		}
		if best < 0 {
			loose = append(loose, s)
			continue
		}
		attach(best, s, bestLang)
	}
	var media []int
	for i := range items {
		if IsMedia(items[i].Kind) {
			media = append(media, i)
		}
	}
	for _, s := range loose {
		switch {
		case len(media) == 0:
			items = append(items, mk(s)) // nothing to attach to here: a plain text file
		case len(media) == 1:
			if lang, ok := GenericSubtitleLang(stemOf(s.name)); ok {
				attach(media[0], s, lang) // "english.srt" beside the one video
				continue
			}
			fallthrough
		default:
			strays = append(strays, path.Join(dirRel, s.name))
		}
	}
	for _, f := range stamps {
		stem, _ := ChapterStem(f.name)
		var hits []int
		for i := range items {
			if IsMedia(items[i].Kind) && chapterFits(stem, items[i].Name) {
				hits = append(hits, i)
			}
		}
		chs, ok := readChapters(f)
		if len(hits) == 0 || !ok {
			items = append(items, mk(f)) // belongs to nothing here, or holds no times: shown as a text file
			continue
		}
		for _, i := range hits {
			if items[i].Chapters == nil { // the first file found wins
				items[i].Chapters = chs
			}
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return NaturalCompare(items[i].Name, items[j].Name) < 0 })
	return items, strays
}

// ---- course.json ----

type courseMeta struct {
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Order       []string `json:"order"`
}

// readMeta reads the optional course.json: {"title","description","order":[folder names]}.
// A broken file is ignored.
func (w *walker) readMeta(courseDir string) courseMeta {
	p := filepath.Join(courseDir, "course.json")
	real, err := filepath.EvalSymlinks(p)
	if err != nil || (!w.follow && !fsutil.Within(w.rootReal, real)) {
		return courseMeta{}
	}
	f, err := os.Open(real)
	if err != nil {
		return courseMeta{}
	}
	defer f.Close()
	var m courseMeta
	if err := json.NewDecoder(io.LimitReader(f, maxMetaFile)).Decode(&m); err != nil {
		w.log.Warn("scan: ignoring unreadable course.json", "path", p, "err", err)
		return courseMeta{}
	}
	m.Title = trimTo(strings.TrimSpace(m.Title), 300)
	m.Description = trimTo(strings.TrimSpace(m.Description), 4000)
	return m
}

func trimTo(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n])
	}
	return s
}

// applySectionOrder moves the sections named in course.json "order" to the
// front, in that order; the rest keep their natural order.
func applySectionOrder(secs []ScannedSection, order []string) {
	if len(order) == 0 {
		return
	}
	rank := map[string]int{}
	for i, name := range order {
		key := strings.ToLower(strings.TrimSpace(name))
		if _, dup := rank[key]; !dup {
			rank[key] = i
		}
	}
	pos := func(s ScannedSection) int {
		if r, ok := rank[strings.ToLower(s.RelPath)]; ok {
			return r
		}
		if r, ok := rank[strings.ToLower(s.Title)]; ok {
			return r
		}
		return len(order)
	}
	sort.SliceStable(secs, func(i, j int) bool { return pos(secs[i]) < pos(secs[j]) })
}
