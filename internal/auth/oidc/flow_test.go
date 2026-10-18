package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/config"
)

type fakeIdP struct {
	t         *testing.T
	srv       *httptest.Server
	key       *rsa.PrivateKey
	challenge string
	claims    map[string]any
}

func newFakeIdP(t *testing.T) *fakeIdP {
	key, _ := rsa.GenerateKey(rand.Reader, 2048)
	idp := &fakeIdP{t: t, key: key}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer": idp.srv.URL, "authorization_endpoint": idp.srv.URL + "/authorize", "token_endpoint": idp.srv.URL + "/token",
			"jwks_uri": idp.srv.URL + "/jwks", "id_token_signing_alg_values_supported": []string{"RS256"},
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "k1", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("/token", idp.token)
	idp.srv = httptest.NewServer(mux)
	t.Cleanup(idp.srv.Close)
	return idp
}

func (i *fakeIdP) token(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	sum := sha256.Sum256([]byte(r.PostForm.Get("code_verifier")))
	if base64.RawURLEncoding.EncodeToString(sum[:]) != i.challenge || r.PostForm.Get("code") != "good-code" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	signer, _ := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: i.key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "k1"))
	payload, _ := json.Marshal(i.claims)
	sig, _ := signer.Sign(payload)
	raw, _ := sig.CompactSerialize()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "at", "token_type": "Bearer", "id_token": raw, "expires_in": 3600})
}

func (i *fakeIdP) baseClaims(nonce string, exp time.Time) map[string]any {
	return map[string]any{"iss": i.srv.URL, "aud": "router", "sub": "user-1", "email": "ada@example.com", "name": "Ada Lovelace",
		"nonce": nonce, "exp": exp.Unix(), "iat": time.Now().Add(-time.Minute).Unix()}
}

type started struct {
	cookie *http.Cookie
	state  string
	nonce  string
}

func begin(t *testing.T, f *Flow, idp *fakeIdP) started {
	w := httptest.NewRecorder()
	if err := f.Begin(w, httptest.NewRequest(http.MethodGet, "/auth/oidc/login?return_to=/routes", nil)); err != nil {
		t.Fatal(err)
	}
	loc, _ := url.Parse(w.Header().Get("Location"))
	q := loc.Query()
	if w.Code != http.StatusFound || q.Get("code_challenge_method") != "S256" || q.Get("client_id") != "router" {
		t.Fatalf("begin redirect: %d %s", w.Code, loc)
	}
	idp.challenge = q.Get("code_challenge")
	return started{cookie: w.Result().Cookies()[0], state: q.Get("state"), nonce: q.Get("nonce")}
}

func complete(f *Flow, s started, state, code string) (auth.Identity, error) {
	r := httptest.NewRequest(http.MethodGet, "/auth/oidc/callback?state="+url.QueryEscape(state)+"&code="+code, nil)
	if s.cookie != nil {
		r.AddCookie(s.cookie)
	}
	return f.Complete(httptest.NewRecorder(), r)
}

func newFlow(t *testing.T, idp *fakeIdP) *Flow {
	sealer, _ := NewStateSealer("cookie-secret")
	cfg := config.OIDCConfig{Issuer: idp.srv.URL, ClientID: "router", ClientSecret: "s", RedirectURL: "http://app/auth/oidc/callback", Scopes: []string{"openid", "email"}}
	return NewFlow(cfg, NewDiscovery(idp.srv.URL, time.Now), sealer, auth.NewCookieJar(false, time.Hour), time.Now)
}

func TestOIDCFlow(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(s *started, claims map[string]any)
		code     string
		wantCode FailureCode
	}{
		{"happy path", func(*started, map[string]any) {}, "good-code", ""},
		{"within clock leeway", func(_ *started, c map[string]any) { c["exp"] = time.Now().Add(-30 * time.Second).Unix() }, "good-code", ""},
		{"state mismatch", func(s *started, _ map[string]any) { s.state = "other" }, "good-code", FailureState},
		{"missing state cookie", func(s *started, _ map[string]any) { s.cookie = nil }, "good-code", FailureState},
		{"bad nonce", func(_ *started, c map[string]any) { c["nonce"] = "evil" }, "good-code", FailureInvalidToken},
		{"expired token", func(_ *started, c map[string]any) { c["exp"] = time.Now().Add(-5 * time.Minute).Unix() }, "good-code", FailureInvalidToken},
		{"wrong audience", func(_ *started, c map[string]any) { c["aud"] = "someone-else" }, "good-code", FailureInvalidToken},
		{"missing email", func(_ *started, c map[string]any) { delete(c, "email") }, "good-code", FailureInvalidToken},
		{"code rejected", func(*started, map[string]any) {}, "bad-code", FailureIdPUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idp := newFakeIdP(t)
			f := newFlow(t, idp)
			s := begin(t, f, idp)
			idp.claims = idp.baseClaims(s.nonce, time.Now().Add(time.Hour))
			tc.mutate(&s, idp.claims)
			id, err := complete(f, s, s.state, tc.code)
			if tc.wantCode == "" {
				if err != nil || id.Email != "ada@example.com" || id.Subject != "user-1" || id.ReturnTo != "/routes" || id.DisplayName != "Ada Lovelace" {
					t.Fatalf("identity %+v err %v", id, err)
				}
				return
			}
			var le *LoginError
			if !errors.As(err, &le) || le.Code != tc.wantCode {
				t.Fatalf("want %s, got %v", tc.wantCode, err)
			}
		})
	}
}

func TestOIDCBeginRejectsOpenRedirect(t *testing.T) {
	idp := newFakeIdP(t)
	f := newFlow(t, idp)
	for _, rt := range []string{"https://evil.example", "//evil.example", "/\\evil"} {
		r := httptest.NewRequest(http.MethodGet, "/auth/oidc/login?return_to="+url.QueryEscape(rt), nil)
		if err := f.Begin(httptest.NewRecorder(), r); !errors.Is(err, ErrBadReturnTo) {
			t.Errorf("%s: %v", rt, err)
		}
	}
}

func TestOIDCBeginIdPDown(t *testing.T) {
	sealer, _ := NewStateSealer("x")
	f := NewFlow(config.OIDCConfig{ClientID: "router"}, NewDiscovery("http://127.0.0.1:1", time.Now), sealer, auth.NewCookieJar(false, time.Hour), time.Now)
	var le *LoginError
	if err := f.Begin(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/auth/oidc/login", nil)); !errors.As(err, &le) || le.Code != FailureIdPUnavailable {
		t.Fatalf("got %v", err)
	}
}
