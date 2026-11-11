package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
)

// Settings are per-user preferences stored as one JSON document.
type Settings struct {
	// ResumeMode: "ask" shows a "continue where you left off?" prompt,
	// "auto" resumes silently, "never" always starts from the beginning.
	ResumeMode   string  `json:"resume_mode"`
	AutoplayNext bool    `json:"autoplay_next"`
	PlaybackRate float64 `json:"playback_rate"`
	// DoneOnOpen marks images, text, links and other non-media items as done
	// as soon as they are opened.
	DoneOnOpen bool `json:"done_on_open"`
}

func DefaultSettings() Settings {
	return Settings{ResumeMode: "ask", AutoplayNext: true, PlaybackRate: 1, DoneOnOpen: true}
}

func (st Settings) Validate() error {
	switch st.ResumeMode {
	case "ask", "auto", "never":
	default:
		return fmt.Errorf("resume_mode must be ask, auto or never")
	}
	if st.PlaybackRate < 0.25 || st.PlaybackRate > 4 {
		return fmt.Errorf("playback_rate must be between 0.25 and 4")
	}
	return nil
}

func (s *Store) GetSettings(ctx context.Context, userID uint64) (Settings, error) {
	st := DefaultSettings()
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT data FROM user_settings WHERE user_id = ?`, userID).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	// Unmarshal over the defaults so keys added later get their default.
	if err := json.Unmarshal(raw, &st); err != nil {
		return DefaultSettings(), nil
	}
	if st.Validate() != nil {
		return DefaultSettings(), nil
	}
	return st, nil
}

func (s *Store) PutSettings(ctx context.Context, userID uint64, st Settings) error {
	if err := st.Validate(); err != nil {
		return err
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	now := nowUTC()
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO user_settings (user_id, data, updated_at) VALUES (?,?,?)
		 ON DUPLICATE KEY UPDATE data = ?, updated_at = ?`, userID, string(b), now, string(b), now)
	return err
}
