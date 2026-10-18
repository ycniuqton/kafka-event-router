package http

import (
	"net/http"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/rbac"
)

// GetSettingsHandler serves GET /workspace-settings (any member; the UI reads the AI-assist switch).
type GetSettingsHandler struct{ d Deps }

func (h *GetSettingsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathSettings), Permission: rbac.PermTeamRead.Auth()}
}

func (h *GetSettingsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	s, err := h.d.Settings.Get(r.Context())
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, s.RowVersion, s)
}

// PatchSettingsHandler serves PATCH /workspace-settings (workspace admins, If-Match required).
type PatchSettingsHandler struct{ d Deps }

func (h *PatchSettingsHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPatch, Pattern: string(PathSettings), Permission: rbac.PermSettingsManage.Auth(),
		Mutating: true, AuditAction: audit.ActionSettingsUpdated, Versioned: true}
}

func (h *PatchSettingsHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	version, err := api.Precondition{}.Require(r)
	if err != nil {
		return err
	}
	patch, err := api.Decoder[rbac.SettingsPatch]{}.Decode(w, r)
	if err != nil {
		return err
	}
	s, err := h.d.Settings.Update(r.Context(), patch, version)
	if err != nil {
		return err
	}
	return h.d.Responder.Versioned(w, http.StatusOK, s.RowVersion, s)
}
