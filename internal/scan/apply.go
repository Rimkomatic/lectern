package scan

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"time"

	"lectern/internal/store"
)

// Counts summarises what a scan found and changed.
type Counts struct {
	Courses        int `json:"courses"`
	Items          int `json:"items"`
	CoursesNew     int `json:"courses_new"`
	CoursesRenamed int `json:"courses_renamed"`
	CoursesMissing int `json:"courses_missing"`
	CoursesFailed  int `json:"courses_failed"`
	ItemsAdded     int `json:"items_added"`
	ItemsUpdated   int `json:"items_updated"`
	ItemsMoved     int `json:"items_moved"`
	ItemsMissing   int `json:"items_missing"`
}

func (c *Counts) Add(o Counts) {
	c.Courses += o.Courses
	c.Items += o.Items
	c.CoursesNew += o.CoursesNew
	c.CoursesRenamed += o.CoursesRenamed
	c.CoursesMissing += o.CoursesMissing
	c.CoursesFailed += o.CoursesFailed
	c.ItemsAdded += o.ItemsAdded
	c.ItemsUpdated += o.ItemsUpdated
	c.ItemsMoved += o.ItemsMoved
	c.ItemsMissing += o.ItemsMissing
}

// Applier compares a folder walk with the database and writes the difference.
type Applier struct {
	Store  *store.Store
	Log    *slog.Logger
	Follow bool
}

func (a *Applier) logger() *slog.Logger {
	if a.Log != nil {
		return a.Log
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// ScanLibrary walks one library and brings the database in line with it.
//
// Safety rules:
//   - nothing is ever deleted here: vanished courses/sections/items are only
//     flagged "missing" (and purged much later), so progress survives;
//   - an unreadable library, or an empty one while courses are known, changes
//     nothing (an unmounted drive must not wipe everybody's progress);
//   - a course folder that cannot be read completely is left exactly as it is.
func (a *Applier) ScanLibrary(ctx context.Context, lib store.Library, report func(done, total int, current string)) (Counts, error) {
	var cnt Counts
	log := a.logger()

	scanned, err := Walk(ctx, lib.Path, WalkOptions{FollowSymlinks: a.Follow, Log: log})
	if err != nil {
		return cnt, err
	}
	dbCourses, err := a.Store.LibraryCourses(ctx, lib.ID)
	if err != nil {
		return cnt, err
	}
	byHash := make(map[string]store.DBCourse, len(dbCourses))
	live := 0
	for _, c := range dbCourses {
		byHash[string(c.PathHash)] = c
		if c.MissingAt == nil {
			live++
		}
	}
	if len(scanned) == 0 && live > 0 {
		return cnt, fmt.Errorf("library looks empty but %d courses are known; leaving them untouched (is the drive mounted?)", live)
	}

	existing := make([]uint64, len(scanned))
	matched := map[uint64]bool{}
	var fresh []int
	for i := range scanned {
		if dc, ok := byHash[string(store.PathHash(scanned[i].RelPath))]; ok {
			existing[i] = dc.ID
			matched[dc.ID] = true
		} else if scanned[i].Err == nil && len(scanned[i].Sections) > 0 {
			fresh = append(fresh, i)
		}
	}
	renamed := map[int]bool{}
	if len(fresh) > 0 {
		var vanished []store.DBCourse
		for _, dc := range dbCourses {
			if !matched[dc.ID] {
				vanished = append(vanished, dc)
			}
		}
		taken := map[uint64]bool{}
		for si, id := range a.detectRenames(ctx, scanned, fresh, vanished) {
			existing[si] = id
			renamed[si] = true
			taken[id] = true
		}
		// A course with a single lesson has too little to compare by path: it is
		// recognised by its file's content instead.
		var rest []int
		for _, si := range fresh {
			if !renamed[si] {
				rest = append(rest, si)
			}
		}
		var gone []store.DBCourse
		for _, dc := range vanished {
			if !taken[dc.ID] {
				gone = append(gone, dc)
			}
		}
		for si, id := range a.detectLoneRenames(ctx, scanned, rest, gone) {
			existing[si] = id
			renamed[si] = true
		}
	}

	seen := map[uint64]bool{}
	total := len(scanned)
	for i := range scanned {
		if err := ctx.Err(); err != nil {
			return cnt, err
		}
		sc := &scanned[i]
		if report != nil {
			report(i, total, sc.Title)
		}
		if sc.Err != nil {
			log.Warn("scan: course folder not fully readable, keeping it as is", "course", sc.RelPath, "err", sc.Err)
			cnt.CoursesFailed++
			if existing[i] != 0 {
				seen[existing[i]] = true
			}
			continue
		}
		if len(sc.Sections) == 0 {
			continue // nothing recognisable inside
		}
		id, st, err := a.syncCourse(ctx, lib, sc, existing[i])
		if err != nil {
			if ctx.Err() != nil {
				return cnt, ctx.Err()
			}
			log.Error("scan: could not store course", "course", sc.RelPath, "err", err)
			cnt.CoursesFailed++
			if existing[i] != 0 {
				seen[existing[i]] = true
			}
			continue
		}
		seen[id] = true
		cnt.Courses++
		cnt.Items += sc.ItemCount()
		if st.CourseNew && !renamed[i] {
			cnt.CoursesNew++
		}
		if renamed[i] {
			cnt.CoursesRenamed++
		}
		cnt.ItemsAdded += st.ItemsAdded
		cnt.ItemsUpdated += st.ItemsUpdated
		cnt.ItemsMoved += st.ItemsMoved
		cnt.ItemsMissing += st.ItemsMissing
	}
	if report != nil {
		report(total, total, "")
	}

	n, err := a.Store.MarkCoursesMissing(ctx, lib.ID, seen)
	if err != nil {
		return cnt, err
	}
	cnt.CoursesMissing = n
	if err := a.Store.SetLibraryScanned(ctx, lib.ID, time.Now().UTC()); err != nil {
		return cnt, err
	}
	return cnt, nil
}

// detectRenames pairs new course folders with courses that vanished from the
// same library when most of their files (by path inside the course) match, so
// that renaming a course folder keeps everyone's progress.
func (a *Applier) detectRenames(ctx context.Context, scanned []ScannedCourse, fresh []int, vanished []store.DBCourse) map[int]uint64 {
	out := map[int]uint64{}
	if len(fresh) == 0 || len(vanished) == 0 || len(fresh)*len(vanished) > 40000 {
		return out
	}
	dbSets := make(map[uint64]map[string]struct{}, len(vanished))
	for _, dc := range vanished {
		if set, err := a.Store.CourseItemHashes(ctx, dc.ID); err == nil && len(set) > 0 {
			dbSets[dc.ID] = set
		}
	}
	type cand struct {
		si     int
		id     uint64
		common int
		ratio  float64
	}
	var cands []cand
	for _, si := range fresh {
		newSet := map[string]struct{}{}
		for _, sec := range scanned[si].Sections {
			for _, it := range sec.Items {
				newSet[string(store.PathHash(it.RelPath))] = struct{}{}
			}
		}
		for id, set := range dbSets {
			common := 0
			for h := range newSet {
				if _, ok := set[h]; ok {
					common++
				}
			}
			if common < 2 {
				continue
			}
			if ratio := float64(common) / float64(max(len(newSet), len(set))); ratio >= 0.6 {
				cands = append(cands, cand{si, id, common, ratio})
			}
		}
	}
	sort.Slice(cands, func(i, j int) bool {
		x, y := cands[i], cands[j]
		switch {
		case x.ratio != y.ratio:
			return x.ratio > y.ratio
		case x.common != y.common:
			return x.common > y.common
		case x.si != y.si:
			return x.si < y.si
		}
		return x.id < y.id
	})
	usedSI, usedID := map[int]bool{}, map[uint64]bool{}
	for _, c := range cands {
		if usedSI[c.si] || usedID[c.id] {
			continue
		}
		usedSI[c.si], usedID[c.id] = true, true
		out[c.si] = c.id
	}
	return out
}

// detectLoneRenames pairs new courses that hold exactly one lesson (a video in
// the library root, or a folder with one file) with vanished courses that held
// exactly one, when the file is the same: same size and fingerprint (or, for
// files not fingerprinted yet, same size and modification time). Renaming such
// a video, or moving it into a folder, then keeps its progress, cover and chapters.
func (a *Applier) detectLoneRenames(ctx context.Context, scanned []ScannedCourse, fresh []int, vanished []store.DBCourse) map[int]uint64 {
	out := map[int]uint64{}
	var cand []int
	for _, si := range fresh {
		if scanned[si].ItemCount() == 1 {
			cand = append(cand, si)
		}
	}
	if len(cand) == 0 || len(vanished) == 0 || len(cand)*len(vanished) > 40000 {
		return out
	}
	type old struct {
		id   uint64
		item store.DBItem
	}
	var olds []old
	for _, dc := range vanished {
		if items, err := a.Store.CourseItems(ctx, dc.ID); err == nil && len(items) == 1 {
			olds = append(olds, old{dc.ID, items[0]})
		}
	}
	used := map[uint64]bool{}
	for _, si := range cand {
		if ctx.Err() != nil {
			break
		}
		it := &scanned[si].Sections[0].Items[0]
		if it.Size == 0 {
			continue
		}
		var fp []byte
		fpDone := false
		for _, o := range olds {
			if used[o.id] || o.item.Size != it.Size {
				continue
			}
			same := false
			if HasFingerprint(o.item.Fingerprint) {
				if !fpDone {
					fp, _ = Fingerprint(it.abs, it.Size)
					fpDone = true
				}
				same = fp != nil && bytes.Equal(fp, o.item.Fingerprint)
			} else {
				same = o.item.Mtime != nil && o.item.Mtime.Equal(it.Mtime)
			}
			if same {
				out[si], used[o.id] = o.id, true
				break
			}
		}
	}
	return out
}

func (a *Applier) syncCourse(ctx context.Context, lib store.Library, sc *ScannedCourse, existingID uint64) (id uint64, st store.SyncStats, err error) {
	req, err := a.buildSync(ctx, lib, sc, existingID)
	if err != nil {
		return 0, st, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		id, st, err = a.Store.SyncCourse(ctx, req)
		if err == nil || !store.IsRetryable(err) {
			return id, st, err
		}
		select {
		case <-ctx.Done():
			return 0, st, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 200 * time.Millisecond):
		}
	}
	return id, st, err
}

// buildSync converts a scanned course into the store's sync request. It
// matches items to existing rows by path, then recognises moved/renamed files
// among the leftovers by size + fingerprint, and probes durations for new or
// changed media.
func (a *Applier) buildSync(ctx context.Context, lib store.Library, sc *ScannedCourse, existingID uint64) (store.SyncCourse, error) {
	req := store.SyncCourse{
		LibraryID:   lib.ID,
		ExistingID:  existingID,
		RelPath:     sc.RelPath,
		Title:       sc.Title,
		Description: sc.Description,
		CoverRel:    sc.CoverRel,
		DirMtime:    sc.DirMtime,
		Sections:    make([]store.SyncSection, len(sc.Sections)),
	}

	var dbItems []store.DBItem
	byHash := map[string]store.DBItem{}
	if existingID != 0 {
		var err error
		if dbItems, err = a.Store.CourseItems(ctx, existingID); err != nil {
			return req, err
		}
		for _, it := range dbItems {
			byHash[string(it.PathHash)] = it
		}
	}

	type ref struct{ si, ii int }
	var fresh []ref
	used := map[uint64]bool{}
	for si, sec := range sc.Sections {
		out := store.SyncSection{RelPath: sec.RelPath, Title: sec.Title, Ord: si, Items: make([]store.SyncItem, len(sec.Items))}
		for ii, it := range sec.Items {
			sit := store.SyncItem{
				RelPath: it.RelPath, Name: it.Name, Title: it.Title, Kind: it.Kind, Ext: it.Ext, Mime: it.Mime,
				Size: it.Size, Mtime: it.Mtime, LinkURL: it.LinkURL, Ord: ii,
			}
			for _, s := range it.Subtitles {
				sit.Subtitles = append(sit.Subtitles, store.SyncSubtitle{RelPath: s.RelPath, Lang: s.Lang, Label: s.Label, Format: s.Format})
			}
			for _, c := range it.Chapters {
				sit.Chapters = append(sit.Chapters, store.ChapterDef{Start: c.Start, Title: c.Title})
			}
			if db, ok := byHash[string(store.PathHash(it.RelPath))]; ok {
				sit.ExistingID = db.ID
				used[db.ID] = true
				sit.Changed = db.Size != it.Size || db.Mtime == nil || !db.Mtime.Equal(it.Mtime)
			} else {
				fresh = append(fresh, ref{si, ii})
			}
			out.Items[ii] = sit
		}
		req.Sections[si] = out
	}

	// Files that are new by path but identical to a file that vanished from
	// this course were renamed or moved: reuse the old row.
	if len(fresh) > 0 && len(dbItems) > 0 {
		pool := map[int64][]store.DBItem{}
		for _, db := range dbItems {
			if !used[db.ID] && db.Size > 0 {
				pool[db.Size] = append(pool[db.Size], db)
			}
		}
		if len(pool) > 0 {
			type pending struct {
				r  ref
				fp []byte
			}
			var weak []pending
			take := func(size int64, k int) store.DBItem {
				c := pool[size][k]
				pool[size] = append(pool[size][:k], pool[size][k+1:]...)
				return c
			}
			for _, r := range fresh {
				if err := ctx.Err(); err != nil {
					return req, err
				}
				it := &sc.Sections[r.si].Items[r.ii]
				if len(pool[it.Size]) == 0 {
					continue
				}
				fp, ferr := Fingerprint(it.abs, it.Size)
				if ferr != nil {
					fp = nil
				}
				matchedStrong := false
				if fp != nil {
					for k, c := range pool[it.Size] {
						if HasFingerprint(c.Fingerprint) && bytes.Equal(c.Fingerprint, fp) {
							old := take(it.Size, k)
							sit := &req.Sections[r.si].Items[r.ii]
							sit.MovedFromID, sit.Fingerprint = old.ID, fp
							matchedStrong = true
							break
						}
					}
				}
				if !matchedStrong {
					weak = append(weak, pending{r, fp})
				}
			}
			// Old rows without a usable fingerprint: fall back to size + mtime.
			for _, p := range weak {
				it := &sc.Sections[p.r.si].Items[p.r.ii]
				for k, c := range pool[it.Size] {
					if !HasFingerprint(c.Fingerprint) && c.Mtime != nil && c.Mtime.Equal(it.Mtime) {
						old := take(it.Size, k)
						sit := &req.Sections[p.r.si].Items[p.r.ii]
						sit.MovedFromID, sit.Fingerprint = old.ID, p.fp
						break
					}
				}
			}
		}
	}

	// Durations for new or changed media, read from the container header.
	for si := range req.Sections {
		for ii := range req.Sections[si].Items {
			sit := &req.Sections[si].Items[ii]
			if !IsMedia(sit.Kind) || !(sit.Changed || (sit.ExistingID == 0 && sit.MovedFromID == 0)) {
				continue
			}
			if d, ok := ProbeDuration(sc.Sections[si].Items[ii].abs, sit.Ext); ok {
				sit.Duration = &d
			}
		}
	}
	return req, nil
}
