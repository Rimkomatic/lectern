package accounts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"lectern/internal/auth"
	"lectern/internal/store"
	"lectern/internal/testdb"
)

func TestGeneratePassword(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 300; i++ {
		pw, err := GeneratePassword()
		if err != nil {
			t.Fatal(err)
		}
		if len(pw) != PasswordLength {
			t.Fatalf("length %d, want %d: %q", len(pw), PasswordLength, pw)
		}
		for _, r := range pw {
			if !strings.ContainsRune(passwordAlphabet, r) {
				t.Fatalf("%q has %q, which is not in the alphabet", pw, r)
			}
		}
		if err := auth.ValidatePassword(pw); err != nil {
			t.Fatalf("a generated password must be acceptable: %v", err)
		}
		if seen[pw] {
			t.Fatalf("the same password came up twice: %q", pw)
		}
		seen[pw] = true
	}
}

func TestPasswordAlphabet(t *testing.T) {
	// Characters that are easy to confuse when a password is read off a screen.
	if i := strings.IndexAny(passwordAlphabet, "ilo0ILO1"); i >= 0 {
		t.Fatalf("the alphabet has the look-alike %q", passwordAlphabet[i])
	}
	have := map[rune]bool{}
	for _, r := range passwordAlphabet {
		if have[r] {
			t.Fatalf("the alphabet repeats %q, which would make it more likely", r)
		}
		have[r] = true
	}
	if len(passwordAlphabet) < 50 {
		t.Fatalf("only %d symbols: too few for %d characters", len(passwordAlphabet), PasswordLength)
	}
}

func TestUnknownUserErrorMessage(t *testing.T) {
	none := (&UnknownUserError{Username: "zed"}).Error()
	if !strings.Contains(none, `"zed"`) || !strings.Contains(none, "nobody has an account yet") {
		t.Errorf("no users: %q", none)
	}
	few := (&UnknownUserError{Username: "zed", Known: []string{"ann", "bob"}}).Error()
	if !strings.Contains(few, "ann, bob") {
		t.Errorf("a few users: %q", few)
	}
	var many []string
	for i := 1; i <= 25; i++ {
		many = append(many, fmt.Sprintf("user%02d", i))
	}
	long := (&UnknownUserError{Username: "zed", Known: many}).Error()
	if !strings.Contains(long, "user20") || strings.Contains(long, "user21") || !strings.Contains(long, "and others") {
		t.Errorf("many users must be cut after %d: %q", maxNamesShown, long)
	}
}

func TestResetPassword(t *testing.T) {
	old := auth.Iterations
	auth.Iterations = 1000
	t.Cleanup(func() { auth.Iterations = old })

	conn, _ := testdb.Open(t)
	st, ctx := store.New(conn), context.Background()

	hashOf := func(pw string) string {
		h, err := auth.HashPassword(pw)
		if err != nil {
			t.Fatal(err)
		}
		return h
	}
	alice, err := st.CreateUser(ctx, "alice", "Alice", hashOf("alice old password"), "admin")
	if err != nil {
		t.Fatal(err)
	}
	bob, err := st.CreateUser(ctx, "bob", "Bob", hashOf("bob old password"), "user")
	if err != nil {
		t.Fatal(err)
	}
	session := func(b byte, u *store.User) []byte {
		t.Helper()
		id := bytes.Repeat([]byte{b}, 32)
		if err := st.CreateSession(ctx, id, u.ID, time.Now().Add(time.Hour), "UA", "10.0.0.1"); err != nil {
			t.Fatal(err)
		}
		return id
	}
	a1, a2, b1 := session(1, alice), session(2, alice), session(3, bob)
	signedIn := func(id []byte) bool {
		_, _, err := st.SessionUser(ctx, id)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			t.Fatal(err)
		}
		return err == nil
	}
	passwordIs := func(name, pw string) bool {
		t.Helper()
		u, err := st.UserByUsername(ctx, name)
		if err != nil {
			t.Fatal(err)
		}
		ok, _ := auth.VerifyPassword(u.PwHash, pw)
		return ok
	}

	t.Run("a random password, and everyone else is left alone", func(t *testing.T) {
		res, err := ResetPassword(ctx, st, "alice", "")
		if err != nil {
			t.Fatal(err)
		}
		if !res.Generated || len(res.Password) != PasswordLength || res.Username != "alice" || res.Disabled {
			t.Fatalf("result: %+v", res)
		}
		if !passwordIs("alice", res.Password) || passwordIs("alice", "alice old password") {
			t.Fatal("alice should have the new password and not the old one")
		}
		if signedIn(a1) || signedIn(a2) {
			t.Fatal("alice must be signed out everywhere")
		}
		if !signedIn(b1) || !passwordIs("bob", "bob old password") {
			t.Fatal("bob must not be touched")
		}
	})

	t.Run("a password you choose; the name may be typed any way", func(t *testing.T) {
		res, err := ResetPassword(ctx, st, "  BOB ", "a chosen password")
		if err != nil {
			t.Fatal(err)
		}
		if res.Generated || res.Password != "a chosen password" || res.Username != "bob" {
			t.Fatalf("result: %+v", res)
		}
		if !passwordIs("bob", "a chosen password") || signedIn(b1) {
			t.Fatal("bob should have the chosen password and be signed out")
		}
	})

	t.Run("a password that is too short changes nothing", func(t *testing.T) {
		before, _ := st.UserByUsername(ctx, "alice")
		keep := session(4, alice)
		if _, err := ResetPassword(ctx, st, "alice", "short"); err == nil {
			t.Fatal("a 5-character password must be refused")
		}
		after, _ := st.UserByUsername(ctx, "alice")
		if before.PwHash != after.PwHash || !signedIn(keep) {
			t.Fatal("a refused password must change nothing")
		}
	})

	t.Run("an unknown user lists the real ones", func(t *testing.T) {
		keep := session(5, alice)
		_, err := ResetPassword(ctx, st, "alic", "")
		var unknown *UnknownUserError
		if !errors.As(err, &unknown) {
			t.Fatalf("want an UnknownUserError, got %v", err)
		}
		if strings.Join(unknown.Known, ",") != "alice,bob" || !strings.Contains(err.Error(), "alice, bob") {
			t.Fatalf("known users: %v / %v", unknown.Known, err)
		}
		if !signedIn(keep) {
			t.Fatal("nothing may change for a name that does not exist")
		}
	})

	t.Run("no name at all", func(t *testing.T) {
		if _, err := ResetPassword(ctx, st, "   ", ""); err == nil {
			t.Fatal("an empty name must be refused")
		}
	})

	t.Run("a disabled account is reported", func(t *testing.T) {
		off := true
		if _, err := st.UpdateUser(ctx, bob.ID, store.UserPatch{Disabled: &off}); err != nil {
			t.Fatal(err)
		}
		res, err := ResetPassword(ctx, st, "bob", "")
		if err != nil {
			t.Fatal(err)
		}
		if !res.Disabled {
			t.Fatalf("a disabled account must be flagged: %+v", res)
		}
		if !passwordIs("bob", res.Password) {
			t.Fatal("the password of a disabled account is changed too")
		}
	})
}

func TestResetPasswordWithoutUsers(t *testing.T) {
	conn, _ := testdb.Open(t)
	_, err := ResetPassword(context.Background(), store.New(conn), "admin", "")
	var unknown *UnknownUserError
	if !errors.As(err, &unknown) || len(unknown.Known) != 0 {
		t.Fatalf("want an UnknownUserError with no users, got %v", err)
	}
}
