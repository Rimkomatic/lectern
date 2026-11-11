package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type User struct {
	ID          uint64     `json:"id"`
	Username    string     `json:"username"`
	DisplayName string     `json:"display_name"`
	Role        string     `json:"role"`
	Disabled    bool       `json:"disabled"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at"`
	PwHash      string     `json:"-"`
}

const userCols = `id, username, display_name, pw_hash, role, disabled, created_at, last_login_at`

func scanUser(sc scanner) (*User, error) {
	var u User
	var last sql.NullTime
	if err := sc.Scan(&u.ID, &u.Username, &u.DisplayName, &u.PwHash, &u.Role, &u.Disabled, &u.CreatedAt, &last); err != nil {
		return nil, err
	}
	u.LastLoginAt = timePtr(last)
	return &u, nil
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) UserByID(ctx context.Context, id uint64) (*User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *Store) UserByUsername(ctx context.Context, username string) (*User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE username = ?`, username))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return u, err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (s *Store) CreateUser(ctx context.Context, username, displayName, pwHash, role string) (*User, error) {
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO users (username, display_name, pw_hash, role, created_at) VALUES (?,?,?,?,?)`,
		username, displayName, pwHash, role, nowUTC())
	if err != nil {
		if isDup(err) {
			return nil, ErrConflict
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.UserByID(ctx, uint64(id))
}

// CreateFirstAdmin creates an admin only while the users table is empty.
func (s *Store) CreateFirstAdmin(ctx context.Context, username, displayName, pwHash string) (*User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users FOR UPDATE`).Scan(&n); err != nil {
		if isDeadlock(err) {
			return nil, ErrConflict
		}
		return nil, err
	}
	if n > 0 {
		return nil, ErrConflict
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO users (username, display_name, pw_hash, role, created_at) VALUES (?,?,?, 'admin', ?)`,
		username, displayName, pwHash, nowUTC())
	if err != nil {
		if isDup(err) || isDeadlock(err) {
			return nil, ErrConflict
		}
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		if isDeadlock(err) {
			return nil, ErrConflict
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.UserByID(ctx, uint64(id))
}

func (s *Store) TouchLogin(ctx context.Context, id uint64) error {
	_, err := s.db.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, nowUTC(), id)
	return err
}

func (s *Store) SetPassword(ctx context.Context, id uint64, pwHash string) error {
	res, err := s.db.ExecContext(ctx, `UPDATE users SET pw_hash = ? WHERE id = ?`, pwHash, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		// RowsAffected is 0 when the hash is unchanged; make sure the user exists.
		if _, err := s.UserByID(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

type UserPatch struct {
	DisplayName *string
	Role        *string
	Disabled    *bool
}

func (s *Store) UpdateUser(ctx context.Context, id uint64, p UserPatch) (*User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var role string
	var disabled bool
	err = tx.QueryRowContext(ctx, `SELECT role, disabled FROM users WHERE id = ? FOR UPDATE`, id).Scan(&role, &disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	newRole, newDisabled := role, disabled
	if p.Role != nil {
		newRole = *p.Role
	}
	if p.Disabled != nil {
		newDisabled = *p.Disabled
	}
	if role == "admin" && !disabled && !(newRole == "admin" && !newDisabled) {
		if err := ensureOtherAdmin(ctx, tx, id); err != nil {
			return nil, err
		}
	}

	var sets []string
	var args []any
	if p.DisplayName != nil {
		sets, args = append(sets, "display_name = ?"), append(args, *p.DisplayName)
	}
	if p.Role != nil {
		sets, args = append(sets, "role = ?"), append(args, *p.Role)
	}
	if p.Disabled != nil {
		sets, args = append(sets, "disabled = ?"), append(args, *p.Disabled)
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := tx.ExecContext(ctx, `UPDATE users SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.UserByID(ctx, id)
}

func (s *Store) DeleteUser(ctx context.Context, id uint64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var role string
	var disabled bool
	err = tx.QueryRowContext(ctx, `SELECT role, disabled FROM users WHERE id = ? FOR UPDATE`, id).Scan(&role, &disabled)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if role == "admin" && !disabled {
		if err := ensureOtherAdmin(ctx, tx, id); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

func ensureOtherAdmin(ctx context.Context, tx *sql.Tx, exceptID uint64) error {
	var others int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled = 0 AND id <> ? FOR UPDATE`, exceptID).Scan(&others); err != nil {
		return err
	}
	if others == 0 {
		return ErrLastAdmin
	}
	return nil
}
