package main

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"lectern/internal/auth"
	"lectern/internal/store"
	"lectern/internal/testdb"
)

func TestReadPassword(t *testing.T) {
	for _, tc := range []struct {
		name, in, want string
		fail           bool
	}{
		{"unix line ending", "correct horse\n", "correct horse", false},
		{"windows line ending", "correct horse\r\n", "correct horse", false},
		{"no line ending", "correct horse", "correct horse", false},
		{"spaces are part of it", "  two spaces  \n", "  two spaces  ", false},
		{"only the first line", "first line\nsecond line\n", "first line", false},
		{"nothing", "", "", true},
		{"an empty line", "\n", "", true},
	} {
		got, err := readPassword(strings.NewReader(tc.in))
		if tc.fail != (err != nil) || got != tc.want {
			t.Errorf("%s: got %q, %v; want %q (fail=%v)", tc.name, got, err, tc.want, tc.fail)
		}
	}
	// A runaway input is cut off rather than read for ever; the password rule then refuses it.
	long, err := readPassword(strings.NewReader(strings.Repeat("x", 100_000)))
	if err != nil || len(long) > 4096 {
		t.Errorf("long input: %d bytes, %v", len(long), err)
	}
}

func TestResetPasswordCommand(t *testing.T) {
	old := auth.Iterations
	auth.Iterations = 1000
	t.Cleanup(func() { auth.Iterations = old })

	conn, _ := testdb.Open(t)
	st, ctx := store.New(conn), context.Background()
	if _, err := st.CreateUser(ctx, "alice", "Alice", "x", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateUser(ctx, "bob", "Bob", "x", "user"); err != nil {
		t.Fatal(err)
	}
	run := func(user, password string) (code int, out, errOut string) {
		var o, e bytes.Buffer
		code = resetPassword(ctx, st, user, password, &o, &e)
		return code, o.String(), e.String()
	}

	t.Run("a random password is shown once", func(t *testing.T) {
		code, out, errOut := run("alice", "")
		if code != 0 || errOut != "" {
			t.Fatalf("code %d, stderr %q", code, errOut)
		}
		var shown string
		for _, line := range strings.Split(out, "\n") {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(line), "New password: "); ok {
				shown = rest
			}
		}
		u, _ := st.UserByUsername(ctx, "alice")
		if ok, _ := auth.VerifyPassword(u.PwHash, shown); !ok || shown == "" {
			t.Fatalf("the password on the screen must be the new one; output:\n%s", out)
		}
		if !strings.Contains(out, `"alice"`) || !strings.Contains(out, "signed out") {
			t.Fatalf("output:\n%s", out)
		}
	})

	t.Run("a password you gave is never printed", func(t *testing.T) {
		code, out, _ := run("bob", "my own passphrase")
		if code != 0 || strings.Contains(out, "my own passphrase") || strings.Contains(out, "New password") {
			t.Fatalf("code %d; output:\n%s", code, out)
		}
		u, _ := st.UserByUsername(ctx, "bob")
		if ok, _ := auth.VerifyPassword(u.PwHash, "my own passphrase"); !ok {
			t.Fatal("bob should have the password that was given")
		}
	})

	t.Run("a mistyped name lists the users on stderr", func(t *testing.T) {
		code, out, errOut := run("alicia", "")
		if code != 1 || out != "" || !strings.Contains(errOut, "alice, bob") {
			t.Fatalf("code %d, stdout %q, stderr %q", code, out, errOut)
		}
	})

	t.Run("a too short password is refused", func(t *testing.T) {
		code, out, errOut := run("alice", "short")
		if code != 1 || out != "" || !strings.Contains(errOut, "8 characters") {
			t.Fatalf("code %d, stdout %q, stderr %q", code, out, errOut)
		}
	})

	t.Run("a disabled account is pointed out", func(t *testing.T) {
		u, _ := st.UserByUsername(ctx, "bob")
		off := true
		if _, err := st.UpdateUser(ctx, u.ID, store.UserPatch{Disabled: &off}); err != nil {
			t.Fatal(err)
		}
		code, out, _ := run("bob", "")
		if code != 0 || !strings.Contains(out, "disabled") {
			t.Fatalf("code %d; output:\n%s", code, out)
		}
	})
}
