package bulk

import (
	"context"

	"github.com/eventrouter/router/internal/routes"
)

// ResumeHandler resumes one route.
type ResumeHandler struct {
	State *routes.StateManager
}

func (ResumeHandler) Action() routes.BulkAction { return routes.BulkResume }

func (h ResumeHandler) Apply(ctx context.Context, id routes.RouteID, _ Args) error {
	_, err := h.State.Resume(ctx, id)
	return err
}
