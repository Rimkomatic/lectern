// Package store holds every SQL statement the app runs against MySQL.
package store

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
)

var (
	ErrNotFound  = errors.New("not found")
	ErrConflict  = errors.New("conflict")
	ErrLastAdmin = errors.New("cannot remove the last enabled admin")
)

type Store struct{ db *sql.DB }

func New(db *sql.DB) *Store { return &Store{db: db} }

func (s *Store) DB() *sql.DB { return s.db }

// nowUTC returns the current time truncated to the DATETIME(3) precision used
// by every timestamp column.
func nowUTC() time.Time { return time.Now().UTC().Truncate(time.Millisecond) }

// PathHash is the identity of a path inside its parent (library, course).
func PathHash(p string) []byte {
	sum := sha256.Sum256([]byte(p))
	return sum[:]
}

func mysqlErrNo(err error) uint16 {
	var me *mysql.MySQLError
	if errors.As(err, &me) {
		return me.Number
	}
	return 0
}

func isDup(err error) bool      { return mysqlErrNo(err) == 1062 }
func isDeadlock(err error) bool { return mysqlErrNo(err) == 1213 }

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func idArgs(ids []uint64) []any {
	out := make([]any, len(ids))
	for i, id := range ids {
		out[i] = id
	}
	return out
}

func escapeLike(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}

type scanner interface{ Scan(dest ...any) error }

func timePtr(n sql.NullTime) *time.Time {
	if !n.Valid {
		return nil
	}
	t := n.Time
	return &t
}

func floatPtr(n sql.NullFloat64) *float64 {
	if !n.Valid {
		return nil
	}
	f := n.Float64
	return &f
}

func intPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	i := int(n.Int64)
	return &i
}
