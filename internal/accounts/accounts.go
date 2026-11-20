// Package accounts holds the account operations that the command line offers:
// the ones an operator runs on the server when the web pages cannot help, for
// example choosing a new password for the administrator who forgot theirs.
package accounts

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"lectern/internal/auth"
	"lectern/internal/store"
)

// passwordAlphabet has no look-alikes (no i, l, o, I, L, O, 0 or 1), so a
// password that is read off a screen and typed in somewhere else survives.
const passwordAlphabet = "abcdefghjkmnpqrstuvwxyzABCDEFGHJKMNPQRSTUVWXYZ23456789"

// PasswordLength is the length of a generated password: about 92 bits.
const PasswordLength = 16

// GeneratePassword returns a random password from passwordAlphabet.
func GeneratePassword() (string, error) {
	max := big.NewInt(int64(len(passwordAlphabet)))
	b := make([]byte, PasswordLength)
	for i := range b {
		n, err := rand.Int(rand.Reader, max) // uniform: no modulo bias
		if err != nil {
			return "", err
		}
		b[i] = passwordAlphabet[n.Int64()]
	}
	return string(b), nil
}

// maxNamesShown caps the list of user names in an error message.
const maxNamesShown = 20

// UnknownUserError says there is no account with that name. Known lists the
// names that do exist, so that a typo is easy to spot.
type UnknownUserError struct {
	Username string
	Known    []string
}

func (e *UnknownUserError) Error() string {
	msg := fmt.Sprintf("there is no user named %q", e.Username)
	switch n := len(e.Known); {
	case n == 0:
		msg += " (nobody has an account yet: create the administrator on Lectern's setup page first)"
	case n <= maxNamesShown:
		msg += " (the users are: " + strings.Join(e.Known, ", ") + ")"
	default:
		msg += " (the users include: " + strings.Join(e.Known[:maxNamesShown], ", ") + ", and others)"
	}
	return msg
}

// Reset is what ResetPassword did.
type Reset struct {
	Username  string // the account's name as it is stored
	Password  string // the new password in clear text; this is the only time it is known
	Generated bool   // Lectern chose the password (the caller gave none)
	Disabled  bool   // the account is disabled, so the new password cannot be used yet
}

// ResetPassword gives a user a new password and signs them out on every
// device. With an empty password it picks a random one; otherwise the given
// one has to pass the same rule as a password chosen in the web page.
func ResetPassword(ctx context.Context, st *store.Store, username, password string) (*Reset, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("no user name given")
	}
	generated := password == ""
	if generated {
		var err error
		if password, err = GeneratePassword(); err != nil {
			return nil, err
		}
	} else if err := auth.ValidatePassword(password); err != nil {
		return nil, err
	}

	u, err := st.UserByUsername(ctx, username)
	if errors.Is(err, store.ErrNotFound) {
		return nil, unknownUser(ctx, st, username)
	}
	if err != nil {
		return nil, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	if err := st.ResetPassword(ctx, u.ID, hash); err != nil {
		return nil, err
	}
	return &Reset{Username: u.Username, Password: password, Generated: generated, Disabled: u.Disabled}, nil
}

func unknownUser(ctx context.Context, st *store.Store, username string) error {
	users, err := st.ListUsers(ctx)
	if err != nil {
		return err
	}
	known := make([]string, len(users))
	for i, u := range users {
		known[i] = u.Username
	}
	return &UnknownUserError{Username: username, Known: known}
}
