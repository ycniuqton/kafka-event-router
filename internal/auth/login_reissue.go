package auth

import "net/http"

// Reissue ends every session of the caller and signs it in again under mode with a fresh session
// (after a password change: other devices are signed out, this browser stays signed in).
func (m *LoginManager) Reissue(w http.ResponseWriter, r *http.Request, mode AuthMode) error {
	p := PrincipalFrom(r.Context())
	if err := m.sessions.RevokeAllForUser(r.Context(), p.UserID); err != nil {
		return err
	}
	user := User{ID: p.UserID, Email: p.Email, DisplayName: p.DisplayName}
	sess, err := m.sessions.Create(r.Context(), m.newSession(r, user, mode))
	if err != nil {
		return err
	}
	m.cookies.SetSession(w, sess)
	return nil
}
