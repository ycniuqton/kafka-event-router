package handlers

import (
	"context"
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/api"
)

// MeContributor adds a top-level member to /auth/me (access-control contributes `access`).
type MeContributor interface {
	Key() string
	Contribute(ctx context.Context, p auth.Principal) (any, error)
}

// MeField names the base members of the /auth/me body.
type MeField string

const (
	FieldID          MeField = "id"
	FieldEmail       MeField = "email"
	FieldDisplayName MeField = "display_name"
	FieldInitials    MeField = "initials"
	FieldTeams       MeField = "teams"
	FieldWorkspace   MeField = "workspace"
	FieldCSRFToken   MeField = "csrf_token"
)

// InitialsLen is how many initials are derived from the display name.
type InitialsLen int

const MaxInitials InitialsLen = 2

// MeHandler serves GET /api/v1/auth/me.
type MeHandler struct{ d Deps }

func (h *MeHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathMe), Auth: api.AuthRequired}
}

func (h *MeHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	p := auth.PrincipalFrom(r.Context())
	body := map[string]any{
		string(FieldID):          p.UserID,
		string(FieldEmail):       p.Email,
		string(FieldDisplayName): p.DisplayName,
		string(FieldInitials):    Initials(p.DisplayName),
		string(FieldTeams):       h.teams(p),
		string(FieldWorkspace):   h.d.Config.Workspace,
		string(FieldCSRFToken):   p.CSRFToken,
	}
	for _, c := range h.d.Contributors {
		v, err := c.Contribute(r.Context(), p)
		if err != nil {
			return err
		}
		body[c.Key()] = v
	}
	return h.d.Responder.JSON(w, http.StatusOK, body)
}

func (h *MeHandler) teams(p auth.Principal) []auth.TeamRole {
	if p.Teams == nil {
		return []auth.TeamRole{}
	}
	return p.Teams
}

// Initials returns up to two upper-case initials of a display name ("Ada Lovelace" → "AL").
type Initials string

// String implements fmt.Stringer.
func (i Initials) String() string { return string(i) }

// MarshalText renders the initials for JSON.
func (i Initials) MarshalText() ([]byte, error) {
	var b strings.Builder
	words := strings.Fields(string(i))
	for _, word := range words[:min(len(words), int(MaxInitials))] {
		r, _ := utf8.DecodeRuneInString(word)
		b.WriteRune(unicode.ToUpper(r))
	}
	return []byte(b.String()), nil
}
