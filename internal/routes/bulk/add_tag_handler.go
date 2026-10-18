package bulk

import (
	"context"

	"github.com/eventrouter/router/internal/routes"
)

// AddTagHandler adds a tag to one route.
type AddTagHandler struct {
	Routes *routes.RouteManager
}

func (AddTagHandler) Action() routes.BulkAction { return routes.BulkAddTag }

func (h AddTagHandler) Apply(ctx context.Context, id routes.RouteID, args Args) error {
	_, err := h.Routes.AddTag(ctx, id, args.Tag)
	return err
}
