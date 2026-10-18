package routes

import (
	"context"
	"encoding/json"

	"github.com/eventrouter/router/internal/routes/validation"
)

// ValidateManager answers POST /routes/{id}/validate for a given or the stored draft.
type ValidateManager struct {
	*Core
}

// NewValidateManager builds the manager over the shared core.
func NewValidateManager(core *Core) *ValidateManager {
	return &ValidateManager{Core: core}
}

// Validate runs mode (draft by default) on draft, falling back to the stored draft.
func (m *ValidateManager) Validate(ctx context.Context, id RouteID, draft json.RawMessage, mode validation.ValidationMode) (validation.Result, error) {
	route, err := m.load(ctx, id)
	if err != nil {
		return validation.Result{}, err
	}
	if mode == "" {
		mode = validation.ModeDraft
	}
	if !mode.IsKnown() {
		return validation.Result{}, ErrBadMode
	}
	if draft == nil {
		draft = route.Draft
	}
	if draft == nil {
		return validation.Result{}, ErrNoDraft
	}
	return m.Validator.Run(ctx, validation.Request{Mode: mode, TeamID: string(route.TeamID), RouteID: string(id), RouteName: route.Name, Raw: draft}), nil
}
