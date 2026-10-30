package password

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/eventrouter/router/internal/audit"
)

func TestAssignGeneratesOrChecksPolicy(t *testing.T) {
	st := newMemCreds()
	id := st.addUser("ada@example.com")
	c, ctx := newTestCredentials(st, &memRecorder{}), context.Background()
	if _, err := c.Assign(ctx, id, "short", true); !errors.Is(err, ErrWeakPassword) {
		t.Fatalf("weak: %v", err)
	}
	gen, err := c.Assign(ctx, id, "", true)
	if err != nil || gen.Generated == "" {
		t.Fatalf("generated=%q err=%v", gen.Generated, err)
	}
	cred, _ := st.Get(ctx, id)
	if !cred.MustChange || cred.Subject == "" {
		t.Fatalf("cred %+v", cred)
	}
	if ok, _ := c.Hashers().Verify(cred.Hash, gen.Generated); !ok {
		t.Fatal("generated password must verify")
	}
	given, _ := c.Assign(ctx, id, "a-given-password", true)
	if given.Generated != "" {
		t.Fatal("given passwords are never echoed")
	}
}

func TestChangeOwnPassword(t *testing.T) {
	st, rec := newMemCreds(), &memRecorder{}
	id := st.addUser("ada@example.com")
	c, ctx := newTestCredentials(st, rec), context.Background()
	_, _ = c.Assign(ctx, id, "initial-password-1", true)
	cases := []struct {
		current, next string
		want          error
	}{
		{"wrong-password-xx", "brand-new-password", ErrInvalidCurrentPassword},
		{"initial-password-1", "too-short", ErrWeakPassword},
		{"initial-password-1", "initial-password-1", ErrPasswordReused},
	}
	for _, tc := range cases {
		if err := c.Change(ctx, id, tc.current, tc.next); !errors.Is(err, tc.want) {
			t.Fatalf("%+v: err=%v", tc, err)
		}
	}
	if err := c.Change(ctx, id, "initial-password-1", "brand-new-password"); err != nil {
		t.Fatal(err)
	}
	cred, _ := st.Get(ctx, id)
	if cred.MustChange {
		t.Fatal("a change clears the forced-change flag")
	}
	if ok, _ := c.Hashers().Verify(cred.Hash, "brand-new-password"); !ok {
		t.Fatal("new password must verify")
	}
	if !slices.Equal(rec.actions(), []audit.Action{audit.ActionPasswordChanged}) {
		t.Fatalf("audit %v", rec.actions())
	}
}

func TestChangeWithoutCredentialIsRejected(t *testing.T) {
	st := newMemCreds()
	id := st.addUser("oidc@example.com")
	if err := newTestCredentials(st, &memRecorder{}).Change(context.Background(), id, "x", "brand-new-password"); !errors.Is(err, ErrInvalidCurrentPassword) {
		t.Fatalf("err=%v", err)
	}
}
