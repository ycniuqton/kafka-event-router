package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCSRFMiddleware(t *testing.T) {
	m := NewCSRFMiddleware(NewCookieJar(false, time.Hour))
	session := Principal{Kind: PrincipalUser, SessionID: "s", CSRFToken: "tok"}
	cases := []struct {
		name           string
		method         string
		p              Principal
		header, cookie string
		wantErr        bool
	}{
		{"safe method", http.MethodGet, session, "", "", false},
		{"matching tokens", http.MethodPost, session, "tok", "tok", false},
		{"missing header", http.MethodPost, session, "", "tok", true},
		{"cookie mismatch", http.MethodPatch, session, "tok", "other", true},
		{"session mismatch", http.MethodDelete, session, "other", "other", true},
		{"non-session principal", http.MethodPost, Principal{Kind: PrincipalWorkload}, "", "", false},
		{"anonymous", http.MethodPost, Anonymous, "", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, "/api/v1/x", nil)
			if tc.header != "" {
				r.Header.Set(string(HeaderCSRFToken), tc.header)
			}
			if tc.cookie != "" {
				r.AddCookie(&http.Cookie{Name: string(CookieCSRF), Value: tc.cookie})
			}
			err := m.Check(r.WithContext(WithPrincipal(r.Context(), tc.p)))
			if tc.wantErr != (err != nil) || (err != nil && !errors.Is(err, ErrCSRF)) {
				t.Fatalf("err = %v", err)
			}
		})
	}
}

func TestBootstrapAuthorizer(t *testing.T) {
	a := NewTeamRoleAuthorizer()
	viewer := Principal{Kind: PrincipalUser, Teams: []TeamRole{{Role: RoleViewer}}}
	editor := Principal{Kind: PrincipalUser, Teams: []TeamRole{{Role: RoleEditor}}}
	read, write, admin := Permission{Key: "x.read", Access: AccessRead}, Permission{Key: "x.edit", Access: AccessWrite}, Permission{Key: "x.admin", Access: AccessAdmin}
	checks := []struct {
		p    Principal
		perm Permission
		ok   bool
	}{
		{viewer, read, true}, {viewer, write, false}, {editor, write, true}, {editor, admin, false},
		{Principal{Kind: PrincipalUser}, read, false}, {Anonymous, read, false},
	}
	for i, c := range checks {
		if err := a.Authorize(t.Context(), c.p, c.perm, ObjectRef{}); (err == nil) != c.ok {
			t.Errorf("case %d: %v", i, err)
		}
	}
}
