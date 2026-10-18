package httpapi

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/dlq"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/httpserver"
)

// Disposition is the Content-Disposition header pattern of exports.
type Disposition string

const (
	headerDisposition Disposition = "Content-Disposition"
	attachment        Disposition = `attachment; filename="%s"`
)

// ExportHandler serves GET /routes/{id}/dlq/export?ids=…|filters&format=json.
type ExportHandler struct{ d Deps }

func (h *ExportHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathExport), Permission: PermAct, Object: RouteObject}
}

// Serve resolves the export, audits it inside its own scope (GET) and streams the file.
func (h *ExportHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	sel, err := ExportSelection{}.Parse(r)
	if err != nil {
		return err
	}
	ctx := audit.WithScope(r.Context(), audit.NewScope(api.AuditContext(r), h.d.Recorder, nil))
	file, err := h.d.Manager.Export(ctx, Caller{}.Viewer(r), chi.URLParam(r, string(ParamRouteID)), sel, dlq.ExportFormat(r.URL.Query().Get(string(QueryFormat))))
	if err != nil {
		return err
	}
	w.Header().Set(string(httpserver.HeaderContentType), file.Exporter.ContentType())
	w.Header().Set(string(headerDisposition), fmt.Sprintf(string(attachment), file.Name))
	w.WriteHeader(http.StatusOK)
	_ = file.Exporter.Write(w, file.Views)
	return nil
}

// ExportSelection parses ?ids=… or the list filters.
type ExportSelection struct{}

// Parse returns an ids selection when ids is given, else a match selection.
func (ExportSelection) Parse(r *http.Request) (dlq.Selection, error) {
	if ids := (Values{}).Split(r.URL.Query().Get(string(QueryIDs))); len(ids) > 0 {
		return dlq.Selection{IDs: ids}, nil
	}
	q, err := QueryParser{}.Parse(r)
	if err != nil {
		return dlq.Selection{}, err
	}
	q = q.WithoutPaging()
	return dlq.Selection{Match: &q}, nil
}
