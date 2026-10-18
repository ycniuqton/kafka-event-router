package bulk

import (
	"context"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/portability"
)

// ExportHandler checks each route can be exported, then encodes all of them into one artifact.
type ExportHandler struct {
	Portability *portability.PortabilityManager
}

func (ExportHandler) Action() routes.BulkAction { return routes.BulkExport }

// Apply fails the ids that cannot be exported (unknown route, nothing to export).
func (h ExportHandler) Apply(ctx context.Context, id routes.RouteID, _ Args) error {
	_, _, err := h.Portability.Document(ctx, id, "")
	return err
}

// Finish encodes the exportable routes in the requested format.
func (h ExportHandler) Finish(ctx context.Context, ids []routes.RouteID, args Args) (*portability.Artifact, error) {
	a, err := h.Portability.Export(ctx, portability.ExportRequest{IDs: ids, Format: args.Format})
	if err != nil {
		return nil, err
	}
	return &a, nil
}
