package http

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/httpserver"
)

// Disposition is the Content-Disposition header of export downloads.
type Disposition string

const (
	HeaderDisposition Disposition = "Content-Disposition"
	AttachmentPattern Disposition = `attachment; filename="%s"`
)

// ExportAuditLogHandler serves GET /audit-log/export?format=jsonl|csv&<filters> as a streamed download.
type ExportAuditLogHandler struct {
	d       Deps
	filters *audit.FilterParser
}

func (h *ExportAuditLogHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathAuditExport), Permission: PermAuditExport}
}

// Serve validates format and filters, audits the export (audit.exported, sensitive) and streams the file.
func (h *ExportAuditLogHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	filter, err := h.filters.Parse(r.URL.Query())
	if err != nil {
		return err
	}
	format := audit.FileFormat(r.URL.Query().Get(string(audit.ParamFormat)))
	stream, err := h.d.Audit.Export(filter, format)
	if err != nil {
		return err
	}
	if err := h.record(r, format); err != nil {
		return err
	}
	w.Header().Set(string(httpserver.HeaderContentType), stream.Exporter.ContentType())
	w.Header().Set(string(HeaderDisposition), fmt.Sprintf(string(AttachmentPattern), stream.FileName(h.d.Now())))
	w.WriteHeader(http.StatusOK)
	_ = stream.WriteTo(r.Context(), w)
	return nil
}

// ExportRecord is the diff stored with audit.exported: the format and the raw filter parameters.
type ExportRecord struct {
	Format audit.FileFormat `json:"format"`
	Filter string           `json:"filter"`
}

func (h *ExportAuditLogHandler) record(r *http.Request, format audit.FileFormat) error {
	diff, err := json.Marshal(ExportRecord{Format: format, Filter: r.URL.RawQuery})
	if err != nil {
		return err
	}
	entry := api.AuditContext(r).Apply(audit.Entry{Action: audit.ActionAuditExported, Object: audit.ObjectAuditLog, Diff: diff})
	return h.d.Recorder.Record(r.Context(), entry)
}

// ExportStatusHandler serves GET /audit-log/export/status (continuous SIEM export progress).
type ExportStatusHandler struct{ d Deps }

func (h *ExportStatusHandler) Route() api.RouteSpec {
	return api.RouteSpec{Method: http.MethodGet, Pattern: string(PathAuditExportStatus), Permission: PermAuditRead}
}

// Serve returns the configured sink, its checkpoint and lag.
func (h *ExportStatusHandler) Serve(w http.ResponseWriter, r *http.Request) error {
	status, err := h.d.Export.Status(r.Context())
	if err != nil {
		return err
	}
	return h.d.Responder.JSON(w, http.StatusOK, status)
}
