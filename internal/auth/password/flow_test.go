package password

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
)

func flowWithUser(t *testing.T, plain string) (*Flow, *memCreds, *fakeThrottle) {
	t.Helper()
	st, th := newMemCreds(), &fakeThrottle{}
	id := st.addUser("Ada@Example.com")
	if _, err := newTestCredentials(st, &memRecorder{}).Assign(context.Background(), id, plain, true); err != nil {
		t.Fatal(err)
	}
	f, err := NewFlow(st, fastHashers(), th)
	if err != nil {
		t.Fatal(err)
	}
	return f, st, th
}

func complete(f *Flow, body string) error {
	r := httptest.NewRequest("POST", "/auth/password/login", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	_, err := f.Complete(httptest.NewRecorder(), r)
	return err
}

func TestFlowSucceedsCaseInsensitiveEmail(t *testing.T) {
	f, _, th := flowWithUser(t, "s3cret-password-ok")
	r := httptest.NewRequest("POST", "/auth/password/login", strings.NewReader(`{"email":" ada@example.com ","password":"s3cret-password-ok"}`))
	id, err := f.Complete(httptest.NewRecorder(), r)
	if err != nil {
		t.Fatal(err)
	}
	if id.Mode != config.AuthModePassword || !strings.HasPrefix(id.Subject, string(PasswordSubjectPrefix)) || id.Email != "Ada@Example.com" {
		t.Fatalf("identity %+v", id)
	}
	if th.cleared != 1 || th.failures != 0 {
		t.Fatalf("throttle %+v", th)
	}
}

func TestFlowRejectsWrongPasswordAndUnknownEmailAlike(t *testing.T) {
	f, _, th := flowWithUser(t, "s3cret-password-ok")
	for _, body := range []string{`{"email":"ada@example.com","password":"nope-nope-nope"}`, `{"email":"bob@example.com","password":"whatever-12chars"}`} {
		if err := complete(f, body); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("%s: err=%v", body, err)
		}
	}
	if th.failures != 2 {
		t.Fatalf("failures=%d", th.failures)
	}
}

func TestFlowLockedSkipsVerification(t *testing.T) {
	f, _, th := flowWithUser(t, "s3cret-password-ok")
	th.locked = &LockedError{RetryAfter: 90 * time.Second}
	err := complete(f, `{"email":"ada@example.com","password":"s3cret-password-ok"}`)
	var locked *LockedError
	if !errors.As(err, &locked) || locked.RetryAfterSeconds() != 90 || locked.ProblemCode() != string(CodeLoginLocked) {
		t.Fatalf("err=%v", err)
	}
	if th.failures != 0 || th.cleared != 0 {
		t.Fatalf("throttle %+v", th)
	}
}

func TestFlowValidatesBody(t *testing.T) {
	f, _, _ := flowWithUser(t, "s3cret-password-ok")
	err := complete(f, `{"email":"","password":""}`)
	var v *api.ValidationError
	if !errors.As(err, &v) || len(v.FieldErrors()) != 2 {
		t.Fatalf("err=%v", err)
	}
}

func TestFlowHasNoBeginStep(t *testing.T) {
	f, _, _ := flowWithUser(t, "s3cret-password-ok")
	if err := f.Begin(nil, nil); err == nil {
		t.Fatal("Begin must not start anything")
	}
}
