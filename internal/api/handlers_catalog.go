package api

import (
	"math"
	"net/http"
	"strings"
	"time"

	"lectern/internal/store"
)

func (s *Server) listCourses(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(q)) > 100 {
		q = string([]rune(q)[:100])
	}
	list, err := s.Store.ListCourses(r.Context(), a.User.ID, q)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, emptyIfNil(list))
	return nil
}

func (s *Server) getCourse(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	tree, err := s.Store.CourseTree(r.Context(), id, a.User.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, tree)
	return nil
}

func (s *Server) getContinue(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	list, err := s.Store.Continue(r.Context(), a.User.ID, 12)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, emptyIfNil(list))
	return nil
}

func (s *Server) getItem(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	d, err := s.Store.ItemDetail(r.Context(), id, a.User.ID)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, d)
	return nil
}

// ---- progress ----

type progressBody struct {
	Event    string   `json:"event"`
	Position *float64 `json:"position"`
	Duration *float64 `json:"duration"`
	Page     *int     `json:"page"`
	Pages    *int     `json:"pages"`
	Done     *bool    `json:"done"`
	ClientTS int64    `json:"client_ts"`
}

var progressEvents = map[string]bool{
	"heartbeat": true, "pause": true, "seek": true, "end": true,
	"open": true, "close": true, "page": true, "done": true,
}

func finiteIn(v *float64, lo, hi float64) bool {
	return v == nil || (!math.IsNaN(*v) && !math.IsInf(*v, 0) && *v >= lo && *v <= hi)
}

func intIn(v *int, lo, hi int) bool { return v == nil || (*v >= lo && *v <= hi) }

// postProgress records where a user is in an item. It is a POST (not PUT) so
// navigator.sendBeacon can use it when the tab closes.
func (s *Server) postProgress(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in progressBody
	if err := decodeJSON(w, r, &in, 4<<10); err != nil {
		return err
	}
	if in.Event == "" {
		in.Event = "heartbeat"
	}
	switch {
	case !progressEvents[in.Event]:
		return badRequest("unknown event")
	case !finiteIn(in.Position, 0, 1e6):
		return badRequest("position is out of range")
	case in.Duration != nil && (!finiteIn(in.Duration, 0, 1e6) || *in.Duration <= 0):
		return badRequest("duration is out of range")
	case !intIn(in.Page, 1, 100000), !intIn(in.Pages, 1, 100000):
		return badRequest("page numbers are out of range")
	}
	now := time.Now().UnixMilli()
	if in.ClientTS <= 0 || in.ClientTS > now+int64(time.Hour/time.Millisecond) {
		in.ClientTS = now // missing, or from a clock far in the future
	}

	loc, err := s.Store.ItemFile(r.Context(), id)
	if err != nil {
		return err
	}
	input := store.ProgressInput{
		Event: in.Event, Position: in.Position, Duration: in.Duration,
		Page: in.Page, Pages: in.Pages, Done: in.Done, ClientTS: in.ClientTS,
	}
	if in.Event == "open" {
		settings, err := s.Store.GetSettings(r.Context(), a.User.ID)
		if err != nil {
			return err
		}
		input.DoneOnOpen = settings.DoneOnOpen
	}
	p, err := s.Store.ApplyProgress(r.Context(), a.User.ID,
		store.ItemRef{ID: loc.ItemID, CourseID: loc.CourseID, Kind: loc.Kind, Chapters: loc.Chapters}, input, s.Cfg.DoneThreshold)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"progress": p})
	return nil
}

// ---- chapters ----

// chaptersBody names chapters by their start in whole seconds, the way the
// item lists them ("start": 754.5 is chapter 754).
type chaptersBody struct {
	Done   []uint32 `json:"done"`
	Undone []uint32 `json:"undone"`
}

// postChapters finishes and un-finishes chapters of a video for the caller. The
// item is done once all of its chapters are. It answers with the caller's
// chapters and the item's progress.
func (s *Server) postChapters(w http.ResponseWriter, r *http.Request, a *authCtx) error {
	id, err := pathID(r, "id")
	if err != nil {
		return err
	}
	var in chaptersBody
	if err := decodeJSON(w, r, &in, 32<<10); err != nil {
		return err
	}
	if len(in.Done)+len(in.Undone) > 2000 {
		return badRequest("too many chapters at once")
	}
	chapters, p, err := s.Store.MarkChapters(r.Context(), a.User.ID, id, in.Done, in.Undone)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, map[string]any{"chapters": chapters, "progress": p})
	return nil
}
