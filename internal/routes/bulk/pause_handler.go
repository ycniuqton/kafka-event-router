package bulk

import (
	"context"

	"github.com/eventrouter/router/internal/routes"
)

// PauseHandler pauses one route.
type PauseHandler struct {
	State *routes.StateManager
}

func (PauseHandler) Action() routes.BulkAction { return routes.BulkPause }

func (h PauseHandler) Apply(ctx context.Context, id routes.RouteID, _ Args) error {
	_, err := h.State.Pause(ctx, id)
	return err
}
