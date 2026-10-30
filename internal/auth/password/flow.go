package password

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/config"
	"github.com/eventrouter/router/internal/store"
)

// LoginRequest is the body of POST /auth/password/login.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// DummySecret is hashed once at start so unknown emails cost the same verification time as known ones.
type DummySecret string

const TimingDummy DummySecret = "timing-equaliser-not-a-password"

// Flow is the password auth.LoginFlow: throttle → credential lookup → constant-time verify → identity.
type Flow struct {
	creds    CredentialStore
	hashers  *HasherSet
	throttle Throttle
	dummy    string
}

// NewFlow builds the flow and precomputes the timing dummy hash.
func NewFlow(creds CredentialStore, hashers *HasherSet, throttle Throttle) (*Flow, error) {
	dummy, err := hashers.Hash(string(TimingDummy))
	if err != nil {
		return nil, err
	}
	return &Flow{creds: creds, hashers: hashers, throttle: throttle, dummy: dummy}, nil
}

// Mode implements auth.LoginFlow.
func (f *Flow) Mode() auth.AuthMode { return config.AuthModePassword }

// Begin is not used by the password flow (single POST).
func (f *Flow) Begin(http.ResponseWriter, *http.Request) error { return auth.ErrLoginModeDisabled }

// Complete verifies the posted email and password and returns the user's identity.
func (f *Flow) Complete(w http.ResponseWriter, r *http.Request) (auth.Identity, error) {
	req, err := f.decode(w, r)
	if err != nil {
		return auth.Identity{}, err
	}
	if err := f.throttle.Admit(r.Context(), r, req.Email); err != nil {
		return auth.Identity{}, err
	}
	cred, err := f.verify(r.Context(), req)
	if errors.Is(err, ErrInvalidCredentials) {
		if ferr := f.throttle.Failed(r.Context(), req.Email); ferr != nil {
			return auth.Identity{}, ferr
		}
	}
	if err != nil {
		return auth.Identity{}, err
	}
	if err := f.throttle.Succeeded(r.Context(), req.Email); err != nil {
		return auth.Identity{}, auth.ErrAuthUnavailable.Wrap(err)
	}
	return auth.Identity{Mode: config.AuthModePassword, Subject: cred.Subject, Email: cred.Email, DisplayName: cred.DisplayName}, nil
}

func (f *Flow) decode(w http.ResponseWriter, r *http.Request) (LoginRequest, error) {
	req, err := api.Decoder[LoginRequest]{}.Decode(w, r)
	if err != nil {
		return req, err
	}
	req.Email = strings.TrimSpace(req.Email)
	v := api.NewValidationError()
	if req.Email == "" {
		v.Add(string(PathEmail), "required", "Enter your email address.")
	}
	if req.Password == "" {
		v.Add(string(PathPassword), "required", "Enter your password.")
	}
	return req, v.OrNil()
}

// verify returns the credential when the password matches; unknown emails verify against the dummy hash.
func (f *Flow) verify(ctx context.Context, req LoginRequest) (Credential, error) {
	cred, err := f.creds.FindByEmail(ctx, req.Email)
	if errors.Is(err, store.ErrNotFound) {
		_, _ = f.hashers.Verify(f.dummy, req.Password)
		return Credential{}, ErrInvalidCredentials
	}
	if err != nil {
		return Credential{}, err
	}
	ok, err := f.hashers.Verify(cred.Hash, req.Password)
	if err != nil {
		return Credential{}, err
	}
	if !ok || cred.Subject == "" {
		return Credential{}, ErrInvalidCredentials
	}
	return cred, nil
}
