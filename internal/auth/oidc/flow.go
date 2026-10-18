package oidc

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"net/http"
	"strings"
	"time"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
)

// QueryParam names the query parameters the flow reads.
type QueryParam string

const (
	ParamReturnTo QueryParam = "return_to"
	ParamState    QueryParam = "state"
	ParamCode     QueryParam = "code"
)

// ClockLeeway tolerates IdP/API clock skew when checking id_token expiry.
type ClockLeeway time.Duration

const DefaultLeeway ClockLeeway = ClockLeeway(60 * time.Second)

// DefaultReturnTo is used when the login did not specify where to go back to.
type DefaultReturnTo string

const RootReturnTo DefaultReturnTo = "/"

// RandomBytes is the entropy of state and nonce values.
type RandomBytes int

const StateBytes RandomBytes = 24

// Flow is the OIDC authorization-code + PKCE login flow.
type Flow struct {
	cfg       config.OIDCConfig
	discovery *Discovery
	sealer    *StateSealer
	cookies   *auth.CookieJar
	now       func() time.Time
}

// NewFlow builds the flow.
func NewFlow(cfg config.OIDCConfig, discovery *Discovery, sealer *StateSealer, cookies *auth.CookieJar, now func() time.Time) *Flow {
	if now == nil {
		now = time.Now
	}
	return &Flow{cfg: cfg, discovery: discovery, sealer: sealer, cookies: cookies, now: now}
}

// Mode implements auth.LoginFlow.
func (f *Flow) Mode() auth.AuthMode {
	return config.AuthModeOIDC
}

// Begin stores state/nonce/PKCE verifier in an encrypted cookie and redirects to the IdP.
func (f *Flow) Begin(w http.ResponseWriter, r *http.Request) error {
	returnTo, err := f.returnTo(r)
	if err != nil {
		return err
	}
	provider, err := f.discovery.Provider(r.Context())
	if err != nil {
		return newLoginError(FailureIdPUnavailable, "identity provider unreachable", err)
	}
	st := LoginState{State: f.random(), Nonce: f.random(), Verifier: oauth2.GenerateVerifier(), ReturnTo: returnTo,
		ExpiresAt: f.now().Add(time.Duration(DefaultStateTTL))}
	sealed, err := f.sealer.Seal(st)
	if err != nil {
		return err
	}
	f.cookies.SetShortLived(w, auth.CookieOIDCState, sealed, time.Duration(DefaultStateTTL))
	url := f.oauth(provider).AuthCodeURL(st.State, gooidc.Nonce(st.Nonce), oauth2.S256ChallengeOption(st.Verifier))
	http.Redirect(w, r, url, http.StatusFound)
	return nil
}

// Complete validates state, exchanges the code with the PKCE verifier and verifies the id_token.
func (f *Flow) Complete(w http.ResponseWriter, r *http.Request) (auth.Identity, error) {
	st, err := f.sealer.Open(f.cookies.Value(r, auth.CookieOIDCState), f.now())
	f.cookies.SetShortLived(w, auth.CookieOIDCState, "", -time.Second)
	if err != nil || st.State != r.URL.Query().Get(string(ParamState)) {
		return auth.Identity{}, newLoginError(FailureState, "login state missing or mismatched", err)
	}
	provider, err := f.discovery.Provider(r.Context())
	if err != nil {
		return auth.Identity{}, newLoginError(FailureIdPUnavailable, "identity provider unreachable", err)
	}
	token, err := f.oauth(provider).Exchange(r.Context(), r.URL.Query().Get(string(ParamCode)), oauth2.VerifierOption(st.Verifier))
	if err != nil {
		return auth.Identity{}, newLoginError(FailureIdPUnavailable, "code exchange failed", err)
	}
	return f.verify(r.Context(), provider, token, st)
}

func (f *Flow) verify(ctx context.Context, provider *gooidc.Provider, token *oauth2.Token, st LoginState) (auth.Identity, error) {
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		return auth.Identity{}, newLoginError(FailureInvalidToken, "id_token missing", nil)
	}
	verifier := provider.Verifier(&gooidc.Config{ClientID: f.cfg.ClientID, Now: f.lenientNow})
	idToken, err := verifier.Verify(ctx, raw)
	if err != nil {
		return auth.Identity{}, newLoginError(FailureInvalidToken, "id_token rejected", err)
	}
	if idToken.Nonce != st.Nonce {
		return auth.Identity{}, newLoginError(FailureInvalidToken, "nonce mismatch", nil)
	}
	var claims Claims
	if err := idToken.Claims(&claims); err != nil || claims.Email == "" {
		return auth.Identity{}, newLoginError(FailureInvalidToken, "email claim missing", err)
	}
	var all map[string]any
	if err := idToken.Claims(&all); err != nil {
		return auth.Identity{}, newLoginError(FailureInvalidToken, "claims unreadable", err)
	}
	return auth.Identity{Mode: config.AuthModeOIDC, Subject: idToken.Subject, Email: claims.Email,
		DisplayName: claims.displayName(), ReturnTo: st.ReturnTo, Groups: ClaimPath(f.cfg.RoleClaims.Claim).Values(all)}, nil
}

func (f *Flow) oauth(provider *gooidc.Provider) *oauth2.Config {
	return &oauth2.Config{ClientID: f.cfg.ClientID, ClientSecret: f.cfg.ClientSecret, RedirectURL: f.cfg.RedirectURL,
		Endpoint: provider.Endpoint(), Scopes: f.cfg.Scopes}
}

func (f *Flow) lenientNow() time.Time {
	return f.now().Add(-time.Duration(DefaultLeeway))
}

func (f *Flow) returnTo(r *http.Request) (string, error) {
	rt := r.URL.Query().Get(string(ParamReturnTo))
	if rt == "" {
		return string(RootReturnTo), nil
	}
	if !strings.HasPrefix(rt, "/") || strings.HasPrefix(rt, "//") || strings.Contains(rt, "\\") {
		return "", ErrBadReturnTo
	}
	return rt, nil
}

func (f *Flow) random() string {
	b := make([]byte, StateBytes)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Claims are the id_token claims the flow uses.
type Claims struct {
	Email             string `json:"email"`
	Name              string `json:"name"`
	PreferredUsername string `json:"preferred_username"`
}

func (c Claims) displayName() string {
	if c.Name != "" {
		return c.Name
	}
	if c.PreferredUsername != "" {
		return c.PreferredUsername
	}
	return c.Email
}
