package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

type Session struct {
	UserID     uint64
	LastSeenAt time.Time
	ExpiresAt  time.Time
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// keep valid UTF-8 when cutting
	r := []rune(s)
	for len(string(r)) > n {
		r = r[:len(r)-1]
	}
	return string(r)
}

func (s *Store) CreateSession(ctx context.Context, idHash []byte, userID uint64, expires time.Time, userAgent, ip string) error {
	now := nowUTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (id, user_id, created_at, last_seen_at, expires_at, user_agent, ip) VALUES (?,?,?,?,?,?,?)`,
		idHash, userID, now, now, expires.UTC(), trunc(userAgent, 255), trunc(ip, 64))
	return err
}

// SessionUser returns the session and its user if the session is unexpired and
// the user is enabled.
func (s *Store) SessionUser(ctx context.Context, idHash []byte) (*Session, *User, error) {
	var sess Session
	var u User
	var last sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT s.user_id, s.last_seen_at, s.expires_at,
		       u.id, u.username, u.display_name, u.pw_hash, u.role, u.disabled, u.created_at, u.last_login_at
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.id = ? AND s.expires_at > ? AND u.disabled = 0`, idHash, nowUTC()).
		Scan(&sess.UserID, &sess.LastSeenAt, &sess.ExpiresAt,
			&u.ID, &u.Username, &u.DisplayName, &u.PwHash, &u.Role, &u.Disabled, &u.CreatedAt, &last)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, ErrNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	u.LastLoginAt = timePtr(last)
	return &sess, &u, nil
}

func (s *Store) TouchSession(ctx context.Context, idHash []byte, newExpires time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ?, expires_at = ? WHERE id = ?`,
		nowUTC(), newExpires.UTC(), idHash)
	return err
}

func (s *Store) DeleteSession(ctx context.Context, idHash []byte) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE id = ?`, idHash)
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID uint64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

// DeleteOtherUserSessions keeps one session (the caller's) and drops the rest.
func (s *Store) DeleteOtherUserSessions(ctx context.Context, userID uint64, keep []byte) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ? AND id <> ?`, userID, keep)
	return err
}

func (s *Store) PurgeExpiredSessions(ctx context.Context) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, nowUTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
