package routeshttp

import (
	"net/http"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/bulk"
)

// BulkBody is the body of POST /routes/bulk.
type BulkBody struct {
	Action string   `json:"action"`
	IDs    []string `json:"ids"`
	Tag    string   `json:"tag"`
	Format string   `json:"format"`
}

// BulkHandler serves POST /routes/bulk; per-id permission checks happen in the managers.
type BulkHandler struct{ d Deps }

func (h *BulkHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodPost, Pattern: string(PathBulk), Permission: PermWrite,
		Mutating: true, AuditAction: routes.ActionRouteUpdated}
}

func (h *BulkHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	body, err := api.Decoder[BulkBody]{}.Decode(w, r)
	if err != nil {
		return err
	}
	ids := make([]routes.RouteID, len(body.IDs))
	for i, id := range body.IDs {
		ids[i] = routes.RouteID(id)
	}
	res, err := h.d.Bulk.Run(r.Context(), bulk.Request{Action: routes.BulkAction(body.Action), IDs: ids, Args: bulk.Args{Tag: body.Tag, Format: routes.DocumentFormat(body.Format)}})
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, res)
}
