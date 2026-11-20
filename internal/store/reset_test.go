package store

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

func TestResetPassword(t *testing.T) {
	s, ctx := newStore(t), context.Background()
	ann, bob := mustUser(t, s, "ann", "user"), mustUser(t, s, "bob", "user")
	exp := time.Now().Add(time.Hour)
	annA, annB, bobA := bytes.Repeat([]byte{1}, 32), bytes.Repeat([]byte{2}, 32), bytes.Repeat([]byte{3}, 32)
	for id, u := range map[*[]byte]*User{&annA: ann, &annB: ann, &bobA: bob} {
		if err := s.CreateSession(ctx, *id, u.ID, exp, "UA", "ip"); err != nil {
			t.Fatal(err)
		}
	}

	if err := s.ResetPassword(ctx, 424242, "x"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown user: %v", err)
	}
	if _, _, err := s.SessionUser(ctx, bobA); err != nil {
		t.Fatalf("a failed reset must not sign anyone out: %v", err)
	}

	if err := s.ResetPassword(ctx, ann.ID, "new-hash"); err != nil {
		t.Fatal(err)
	}
	got, err := s.UserByID(ctx, ann.ID)
	if err != nil || got.PwHash != "new-hash" {
		t.Fatalf("hash: %v %+v", err, got)
	}
	for _, id := range [][]byte{annA, annB} {
		if _, _, err := s.SessionUser(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("ann's session survived: %v", err)
		}
	}
	if _, _, err := s.SessionUser(ctx, bobA); err != nil {
		t.Fatalf("bob was signed out: %v", err)
	}
	if other, _ := s.UserByID(ctx, bob.ID); other.PwHash != "hash-bob" {
		t.Fatalf("bob's password changed: %q", other.PwHash)
	}
}
