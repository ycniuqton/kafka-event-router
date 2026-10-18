// Package http serves the audit log read API: the filtered, cursor-paged log and the action catalog
// the filter bar offers. Only workspace admins (audit.read) may call it.
package http

import (
	"context"
	"time"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/rbac"
)

// Path names the audit log routes.
type Path string

const (
	PathAuditLog          Path = "/api/v1/audit-log"
	PathAuditActions      Path = "/api/v1/audit-log/actions"
	PathAuditExport       Path = "/api/v1/audit-log/export"
	PathAuditExportStatus Path = "/api/v1/audit-log/export/status"
)

// PermAuditRead guards every audit read route (decision: only workspace admins read the audit log).
var PermAuditRead = rbac.PermAuditRead.Auth()

// PermAuditExport guards the file export.
var PermAuditExport = rbac.PermAuditExport.Auth()

// ExportStatusReader reports the continuous SIEM export (audit.ExportMonitor).
type ExportStatusReader interface {
	Status(ctx context.Context) (audit.ExportStatus, error)
}

// Deps groups what the handlers need; Recorder audits file exports, Now stamps their file names.
type Deps struct {
	Audit     audit.Manager
	Responder *api.Responder
	Pages     *api.PageParser
	Recorder  audit.Recorder
	Export    ExportStatusReader
	Now       func() time.Time
}

// NewModule returns the audit log read handlers.
func NewModule(d Deps) api.Module {
	return api.Modules{&ListAuditLogHandler{d: d, filters: audit.NewFilterParser(d.Audit.Catalog())}, &ListAuditActionsHandler{d: d}}
}

// NewExportModule returns the file export and the SIEM export status handlers (Recorder, Export and Now required).
func NewExportModule(d Deps) api.Module {
	return api.Modules{&ExportAuditLogHandler{d: d, filters: audit.NewFilterParser(d.Audit.Catalog())}, &ExportStatusHandler{d: d}}
}
