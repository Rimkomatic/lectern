package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// ---- DTOs (serialised as-is by the API) ----

type CourseSummary struct {
	ID          uint64 `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description"`
	Library     string `json:"library"`
	HasCover    bool   `json:"has_cover"` // an uploaded cover or an image in the folder
	CoverV      int64  `json:"cover_v"`   // version of the uploaded cover; 0 if there is none
	// ItemCount and DoneCount count lessons: every file, except that each
	// chapter of a video with chapters is a lesson of its own.
	ItemCount  int        `json:"item_count"`
	DoneCount  int        `json:"done_count"`
	DurationS  float64    `json:"duration_s"`
	LastSeenAt *time.Time `json:"last_seen_at"`
}

type Subtitle struct {
	ID     uint64 `json:"id"`
	Lang   string `json:"lang"`
	Label  string `json:"label"`
	Format string `json:"format"`
}

type Progress struct {
	State     string     `json:"state"`
	Position  float64    `json:"position"`
	Furthest  float64    `json:"furthest"`
	Duration  *float64   `json:"duration"`
	Page      *int       `json:"page"`
	Pages     *int       `json:"pages"`
	DoneAt    *time.Time `json:"done_at"`
	ClientTS  int64      `json:"-"`
	UpdatedAt time.Time  `json:"updated_at"`
}

type Item struct {
	ID        uint64     `json:"id"`
	CourseID  uint64     `json:"course_id"`
	SectionID uint64     `json:"section_id"`
	Name      string     `json:"name"`
	Title     string     `json:"title"`
	Kind      string     `json:"kind"`
	Ext       string     `json:"ext"`
	Mime      string     `json:"mime"`
	Size      int64      `json:"size"`
	Duration  *float64   `json:"duration"`
	Pages     *int       `json:"pages"`
	LinkURL   string     `json:"link_url,omitempty"`
	Ord       int        `json:"ord"`
	Subtitles []Subtitle `json:"subtitles"`
	Progress  *Progress  `json:"progress"`
	// Chapters of a long video (from "<name>.chapters.txt"), with this user's
	// finished ones marked. Absent for everything else.
	Chapters []Chapter `json:"chapters,omitempty"`
}

type Section struct {
	ID    uint64 `json:"id"`
	Title string `json:"title"`
	Ord   int    `json:"ord"`
	// Loose is true for the implicit section of files that sit directly in the
	// course folder. A course with only that section has no sections to show.
	Loose bool   `json:"loose"`
	Items []Item `json:"items"`
}

type Resume struct {
	ItemID    uint64  `json:"item_id"`
	SectionID uint64  `json:"section_id"`
	Title     string  `json:"title"`
	Kind      string  `json:"kind"`
	Position  float64 `json:"position"`
	Page      *int    `json:"page"`
	Reason    string  `json:"reason"` // "continue" (same item) | "next" (next unfinished item)
}

type CourseTree struct {
	Course    CourseSummary `json:"course"`
	Sections  []Section     `json:"sections"`
	Resume    *Resume       `json:"resume"`
	Completed bool          `json:"completed"`
}

type ItemDetail struct {
	Item         Item   `json:"item"`
	CourseTitle  string `json:"course_title"`
	SectionTitle string `json:"section_title"`
}

type ContinueItem struct {
	ID       uint64   `json:"id"`
	Title    string   `json:"title"`
	Kind     string   `json:"kind"`
	Duration *float64 `json:"duration"`
	State    string   `json:"state"`
	Position float64  `json:"position"`
	Page     *int     `json:"page"`
	Pages    *int     `json:"pages"`
}

type ContinueEntry struct {
	Course CourseSummary `json:"course"`
	Item   *ContinueItem `json:"item"`
}

// FileLoc tells the media layer where a file lives. ItemRel is relative to the
// course directory (for covers and subtitles it is the cover/subtitle path).
type FileLoc struct {
	ItemID      uint64
	CourseID    uint64
	Kind        string
	Name        string
	Ext         string
	Mime        string
	Size        int64
	Mtime       *time.Time
	Format      string // subtitles only
	LibraryPath string
	CourseRel   string
	ItemRel     string
	Chapters    int // number of chapters of the item (0 = none)
}

// ---- course list / tree ----

func (s *Store) courseSummaries(ctx context.Context, userID uint64, extraWhere string, args ...any) ([]CourseSummary, error) {
	// A video with chapters counts once per chapter. Its done chapters are the
	// ones this user finished, or all of them once the item itself is done.
	q := `SELECT c.id, c.title, COALESCE(c.description, ''), l.label,
	             (c.cover_rel IS NOT NULL OR cv.course_id IS NOT NULL), COALESCE(cv.v, 0),
	             CAST(COALESCE(SUM(CASE WHEN i.id IS NULL THEN 0 ELSE COALESCE(ic.n, 1) END), 0) AS SIGNED),
	             CAST(COALESCE(SUM(CASE WHEN i.id IS NULL THEN 0
	                                    WHEN ic.n IS NULL THEN p.state = 'done'
	                                    WHEN p.state = 'done' THEN ic.n
	                                    ELSE COALESCE(cpn.n, 0) END), 0) AS SIGNED),
	             COALESCE(SUM(i.duration), 0), cs.last_seen_at
	      FROM courses c
	      JOIN libraries l ON l.id = c.library_id
	      LEFT JOIN course_covers cv ON cv.course_id = c.id
	      LEFT JOIN items i ON i.course_id = c.id AND i.missing_at IS NULL
	      LEFT JOIN item_chapters ic ON ic.item_id = i.id
	      LEFT JOIN progress p ON p.item_id = i.id AND p.user_id = ?
	      LEFT JOIN (SELECT item_id, COUNT(*) AS n FROM chapter_progress WHERE user_id = ? GROUP BY item_id) cpn
	             ON cpn.item_id = i.id
	      LEFT JOIN course_state cs ON cs.course_id = c.id AND cs.user_id = ?
	      WHERE c.missing_at IS NULL AND l.enabled = 1 ` + extraWhere + `
	      GROUP BY c.id, c.title, c.description, l.label, c.cover_rel, cv.course_id, cv.v, cs.last_seen_at
	      ORDER BY c.title, c.id`
	full := append([]any{userID, userID, userID}, args...)
	rows, err := s.db.QueryContext(ctx, q, full...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CourseSummary{}
	for rows.Next() {
		var c CourseSummary
		var last sql.NullTime
		if err := rows.Scan(&c.ID, &c.Title, &c.Description, &c.Library, &c.HasCover, &c.CoverV,
			&c.ItemCount, &c.DoneCount, &c.DurationS, &last); err != nil {
			return nil, err
		}
		c.LastSeenAt = timePtr(last)
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) ListCourses(ctx context.Context, userID uint64, query string) ([]CourseSummary, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return s.courseSummaries(ctx, userID, "")
	}
	return s.courseSummaries(ctx, userID, "AND c.title LIKE ?", "%"+escapeLike(query)+"%")
}

const itemSelect = `SELECT i.id, i.course_id, i.section_id, i.name, i.title, i.kind, i.ext, i.mime, i.size,
	       i.duration, i.pages, i.link_url, i.ord,
	       p.state, p.position, p.furthest, p.duration, p.page, p.pages, p.done_at, p.updated_at,
	       ic.chapters
	FROM items i
	LEFT JOIN progress p ON p.item_id = i.id AND p.user_id = ?
	LEFT JOIN item_chapters ic ON ic.item_id = i.id `

func scanItem(sc scanner) (Item, error) {
	var it Item
	var dur, pPos, pFur, pDur sql.NullFloat64
	var pages, pPage, pPages sql.NullInt64
	var link, pState, chapters sql.NullString
	var pDone, pUpd sql.NullTime
	if err := sc.Scan(&it.ID, &it.CourseID, &it.SectionID, &it.Name, &it.Title, &it.Kind, &it.Ext, &it.Mime, &it.Size,
		&dur, &pages, &link, &it.Ord,
		&pState, &pPos, &pFur, &pDur, &pPage, &pPages, &pDone, &pUpd, &chapters); err != nil {
		return it, err
	}
	it.Duration = floatPtr(dur)
	it.Pages = intPtr(pages)
	it.LinkURL = link.String
	it.Subtitles = []Subtitle{}
	if pState.Valid {
		it.Progress = &Progress{
			State:     pState.String,
			Position:  pPos.Float64,
			Furthest:  pFur.Float64,
			Duration:  floatPtr(pDur),
			Page:      intPtr(pPage),
			Pages:     intPtr(pPages),
			DoneAt:    timePtr(pDone),
			UpdatedAt: pUpd.Time,
		}
	}
	// Who has finished which chapter is filled in by the caller (withMarks).
	it.Chapters = chaptersOf(parseDefs(chapters), false, nil)
	return it, nil
}

// withMarks sets the done flags of an item's chapters from this user's marks.
func (it *Item) withMarks(marks map[uint32]bool) {
	done := it.Progress != nil && it.Progress.State == "done"
	for i := range it.Chapters {
		it.Chapters[i].Done = done || marks[ChapterKey(it.Chapters[i].Start)]
	}
}

func (s *Store) CourseTree(ctx context.Context, courseID, userID uint64) (*CourseTree, error) {
	sums, err := s.courseSummaries(ctx, userID, "AND c.id = ?", courseID)
	if err != nil {
		return nil, err
	}
	if len(sums) == 0 {
		return nil, ErrNotFound
	}
	tree := &CourseTree{Course: sums[0], Sections: []Section{}}

	// sections
	srows, err := s.db.QueryContext(ctx,
		`SELECT id, title, ord, rel_path = '' FROM sections WHERE course_id = ? AND missing_at IS NULL ORDER BY ord, id`, courseID)
	if err != nil {
		return nil, err
	}
	secIdx := map[uint64]int{}
	for srows.Next() {
		var sec Section
		if err := srows.Scan(&sec.ID, &sec.Title, &sec.Ord, &sec.Loose); err != nil {
			srows.Close()
			return nil, err
		}
		sec.Items = []Item{}
		secIdx[sec.ID] = len(tree.Sections)
		tree.Sections = append(tree.Sections, sec)
	}
	if err := srows.Err(); err != nil {
		srows.Close()
		return nil, err
	}
	srows.Close()

	// items (+ this user's progress)
	irows, err := s.db.QueryContext(ctx,
		itemSelect+`WHERE i.course_id = ? AND i.missing_at IS NULL ORDER BY i.section_id, i.ord, i.id`, userID, courseID)
	if err != nil {
		return nil, err
	}
	itemPos := map[uint64][2]int{} // item id -> [section index, item index]
	for irows.Next() {
		it, err := scanItem(irows)
		if err != nil {
			irows.Close()
			return nil, err
		}
		si, ok := secIdx[it.SectionID]
		if !ok {
			continue
		}
		itemPos[it.ID] = [2]int{si, len(tree.Sections[si].Items)}
		tree.Sections[si].Items = append(tree.Sections[si].Items, it)
	}
	if err := irows.Err(); err != nil {
		irows.Close()
		return nil, err
	}
	irows.Close()

	// chapters finished by this user
	marks, err := s.userMarks(ctx, userID, "i.course_id = ?", courseID)
	if err != nil {
		return nil, err
	}
	for id, pos := range itemPos {
		it := &tree.Sections[pos[0]].Items[pos[1]]
		if len(it.Chapters) > 0 {
			it.withMarks(marks[id])
		}
	}

	// subtitles
	subrows, err := s.db.QueryContext(ctx, `
		SELECT st.id, st.item_id, st.lang, st.label, st.format
		FROM subtitles st JOIN items i ON i.id = st.item_id
		WHERE i.course_id = ? AND i.missing_at IS NULL ORDER BY st.lang, st.id`, courseID)
	if err != nil {
		return nil, err
	}
	for subrows.Next() {
		var st Subtitle
		var itemID uint64
		if err := subrows.Scan(&st.ID, &itemID, &st.Lang, &st.Label, &st.Format); err != nil {
			subrows.Close()
			return nil, err
		}
		if p, ok := itemPos[itemID]; ok {
			it := &tree.Sections[p[0]].Items[p[1]]
			it.Subtitles = append(it.Subtitles, st)
		}
	}
	if err := subrows.Err(); err != nil {
		subrows.Close()
		return nil, err
	}
	subrows.Close()

	// drop empty sections
	kept := tree.Sections[:0]
	for _, sec := range tree.Sections {
		if len(sec.Items) > 0 {
			kept = append(kept, sec)
		}
	}
	tree.Sections = kept

	var last sql.NullInt64
	err = s.db.QueryRowContext(ctx,
		`SELECT last_item_id FROM course_state WHERE user_id = ? AND course_id = ?`, userID, courseID).Scan(&last)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	tree.Resume, tree.Completed = ComputeResume(tree.Sections, uint64(last.Int64))
	return tree, nil
}

func (s *Store) ItemDetail(ctx context.Context, itemID, userID uint64) (*ItemDetail, error) {
	it, err := scanItem(s.db.QueryRowContext(ctx,
		itemSelect+`JOIN courses c ON c.id = i.course_id
		WHERE i.id = ? AND i.missing_at IS NULL AND c.missing_at IS NULL`, userID, itemID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if len(it.Chapters) > 0 {
		marks, err := s.userMarks(ctx, userID, "cp.item_id = ?", itemID)
		if err != nil {
			return nil, err
		}
		it.withMarks(marks[itemID])
	}
	d := &ItemDetail{Item: it}
	if err := s.db.QueryRowContext(ctx,
		`SELECT c.title, sec.title FROM courses c JOIN sections sec ON sec.id = ? WHERE c.id = ?`,
		it.SectionID, it.CourseID).Scan(&d.CourseTitle, &d.SectionTitle); err != nil {
		return nil, err
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, lang, label, format FROM subtitles WHERE item_id = ? ORDER BY lang, id`, itemID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var st Subtitle
		if err := rows.Scan(&st.ID, &st.Lang, &st.Label, &st.Format); err != nil {
			return nil, err
		}
		d.Item.Subtitles = append(d.Item.Subtitles, st)
	}
	return d, rows.Err()
}

// ---- file locations ----

func (s *Store) ItemFile(ctx context.Context, itemID uint64) (*FileLoc, error) {
	var f FileLoc
	var mt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT i.id, i.course_id, i.kind, i.name, i.ext, i.mime, i.size, i.mtime, l.path, c.rel_path, i.rel_path,
		       COALESCE(ic.n, 0)
		FROM items i
		JOIN courses c   ON c.id = i.course_id
		JOIN libraries l ON l.id = c.library_id
		LEFT JOIN item_chapters ic ON ic.item_id = i.id
		WHERE i.id = ? AND i.missing_at IS NULL AND c.missing_at IS NULL AND l.enabled = 1`, itemID).
		Scan(&f.ItemID, &f.CourseID, &f.Kind, &f.Name, &f.Ext, &f.Mime, &f.Size, &mt, &f.LibraryPath, &f.CourseRel, &f.ItemRel,
			&f.Chapters)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	f.Mtime = timePtr(mt)
	return &f, err
}

func (s *Store) SubtitleFile(ctx context.Context, subID uint64) (*FileLoc, error) {
	var f FileLoc
	err := s.db.QueryRowContext(ctx, `
		SELECT st.id, i.course_id, st.format, l.path, c.rel_path, st.rel_path
		FROM subtitles st
		JOIN items i     ON i.id = st.item_id
		JOIN courses c   ON c.id = i.course_id
		JOIN libraries l ON l.id = c.library_id
		WHERE st.id = ? AND i.missing_at IS NULL AND c.missing_at IS NULL AND l.enabled = 1`, subID).
		Scan(&f.ItemID, &f.CourseID, &f.Format, &f.LibraryPath, &f.CourseRel, &f.ItemRel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	f.Kind = "text"
	return &f, err
}

func (s *Store) CoverFile(ctx context.Context, courseID uint64) (*FileLoc, error) {
	var f FileLoc
	err := s.db.QueryRowContext(ctx, `
		SELECT c.id, l.path, c.rel_path, c.cover_rel
		FROM courses c JOIN libraries l ON l.id = c.library_id
		WHERE c.id = ? AND c.cover_rel IS NOT NULL AND c.missing_at IS NULL AND l.enabled = 1`, courseID).
		Scan(&f.CourseID, &f.LibraryPath, &f.CourseRel, &f.ItemRel)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	f.Kind = "image"
	return &f, err
}

// ---- continue-learning row ----

func (s *Store) Continue(ctx context.Context, userID uint64, limit int) ([]ContinueEntry, error) {
	if limit <= 0 {
		limit = 12
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT cs.course_id, i.id, i.title, i.kind, i.duration, p.state, p.position, p.page, p.pages
		FROM course_state cs
		JOIN courses c ON c.id = cs.course_id AND c.missing_at IS NULL
		LEFT JOIN items i ON i.id = cs.last_item_id AND i.missing_at IS NULL
		LEFT JOIN progress p ON p.item_id = i.id AND p.user_id = cs.user_id
		WHERE cs.user_id = ?
		ORDER BY cs.last_seen_at DESC
		LIMIT ?`, userID, limit*3)
	if err != nil {
		return nil, err
	}
	type row struct {
		courseID uint64
		item     *ContinueItem
	}
	var ordered []row
	var ids []uint64
	for rows.Next() {
		var courseID uint64
		var itemID sql.NullInt64
		var title, kind, state sql.NullString
		var dur, pos sql.NullFloat64
		var page, pages sql.NullInt64
		if err := rows.Scan(&courseID, &itemID, &title, &kind, &dur, &state, &pos, &page, &pages); err != nil {
			rows.Close()
			return nil, err
		}
		r := row{courseID: courseID}
		if itemID.Valid {
			r.item = &ContinueItem{
				ID: uint64(itemID.Int64), Title: title.String, Kind: kind.String,
				Duration: floatPtr(dur), State: state.String, Position: pos.Float64,
				Page: intPtr(page), Pages: intPtr(pages),
			}
		}
		ordered = append(ordered, r)
		ids = append(ids, courseID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	if len(ids) == 0 {
		return []ContinueEntry{}, nil
	}

	sums, err := s.courseSummaries(ctx, userID, "AND c.id IN ("+placeholders(len(ids))+")", idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	byID := make(map[uint64]CourseSummary, len(sums))
	for _, c := range sums {
		byID[c.ID] = c
	}
	out := []ContinueEntry{}
	for _, r := range ordered {
		c, ok := byID[r.courseID]
		if !ok || (c.ItemCount > 0 && c.DoneCount >= c.ItemCount) {
			continue // vanished or fully completed
		}
		out = append(out, ContinueEntry{Course: c, Item: r.item})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}
