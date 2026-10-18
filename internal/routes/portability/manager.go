package portability

import (
	"context"
	"encoding/json"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/routes"
)

// FileName names export attachments.
type FileName string

const (
	bulkFileStem FileName = "routes-export"
	extSeparator FileName = "."
)

// ExportRequest selects routes, format and version reference ("", "active", "draft" or a number).
type ExportRequest struct {
	IDs     []routes.RouteID
	Format  routes.DocumentFormat
	Version string
}

// Artifact is an encoded export ready to download.
type Artifact struct {
	Format    routes.DocumentFormat `json:"format"`
	MediaType string                `json:"mediaType"`
	FileName  string                `json:"fileName"`
	Content   string                `json:"content"`
}

// Deps are the collaborators of the PortabilityManager.
type Deps struct {
	Routes    *routes.RouteManager
	Versions  *routes.VersionManager
	Repos     routes.Repositories
	Validator routes.Validator
	Resolver  ConnectionResolver
	Codecs    *CodecManager
	Auditor   routes.RouteAuditor
	Catalog   *problem.Catalog
}

// PortabilityManager exports routes as documents and imports documents as drafts (flow 5.12).
type PortabilityManager struct {
	d Deps
}

// NewPortabilityManager builds the manager.
func NewPortabilityManager(d Deps) *PortabilityManager {
	if d.Catalog == nil {
		d.Catalog = problem.NewCatalog()
	}
	return &PortabilityManager{d: d}
}

// Export encodes the requested routes (connection names instead of ids) into one payload.
func (m *PortabilityManager) Export(ctx context.Context, req ExportRequest) (Artifact, error) {
	if err := m.checkIDs(req.IDs); err != nil {
		return Artifact{}, err
	}
	codec, err := m.d.Codecs.Codec(req.Format)
	if err != nil {
		return Artifact{}, err
	}
	docs := make([]json.RawMessage, 0, len(req.IDs))
	names := make([]string, 0, len(req.IDs))
	for _, id := range req.IDs {
		route, doc, err := m.Document(ctx, id, req.Version)
		if err != nil {
			return Artifact{}, err
		}
		docs, names = append(docs, doc), append(names, route.Name)
		if err := m.recordExport(ctx, route, codec.Format()); err != nil {
			return Artifact{}, err
		}
	}
	content, err := codec.Encode(docs)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{Format: codec.Format(), MediaType: codec.MediaType(), FileName: m.fileName(names, codec.Format()), Content: string(content)}, nil
}

// Document returns one route's portable document (JSON, connection names) for a version reference.
func (m *PortabilityManager) Document(ctx context.Context, id routes.RouteID, version string) (routes.Route, json.RawMessage, error) {
	route, raw, err := m.d.Versions.Config(ctx, id, version)
	if err != nil {
		return routes.Route{}, nil, err
	}
	if raw == nil {
		return routes.Route{}, nil, ErrNothingToExport
	}
	tree, err := ParseTree(raw)
	if err != nil {
		return routes.Route{}, nil, err
	}
	if err := m.d.Resolver.ToNames(ctx, tree); err != nil {
		return routes.Route{}, nil, err
	}
	tree.DropConsumerGroup(routes.ConsumerGroupNamer{}.For(route.Name))
	doc, err := tree.JSON()
	return route, doc, err
}

// checkIDs bounds the number of exported routes.
func (PortabilityManager) checkIDs(ids []routes.RouteID) error {
	if len(ids) == 0 {
		return ErrNoIDs
	}
	if len(ids) > int(MaxIDs) {
		return ErrTooManyIDs
	}
	return nil
}

// fileName is "<route>.<ext>" for one route, else "routes-export.<ext>".
func (PortabilityManager) fileName(names []string, format routes.DocumentFormat) string {
	stem := string(bulkFileStem)
	if len(names) == 1 {
		stem = names[0]
	}
	return stem + string(extSeparator) + string(format)
}

// recordExport audits the export inside request scopes (POST /routes/export, bulk export).
func (m *PortabilityManager) recordExport(ctx context.Context, r routes.Route, format routes.DocumentFormat) error {
	return m.d.Auditor.RecordIfScoped(ctx, routes.AuditRecord{Action: routes.ActionRouteExported, Object: audit.ObjectRoute,
		ObjectID: string(r.ID), Team: r.TeamID, After: ExportAuditView{Name: r.Name, Format: format}})
}

// ExportAuditView is the audited summary of an export.
type ExportAuditView struct {
	Name   string                `json:"name"`
	Format routes.DocumentFormat `json:"format"`
}
