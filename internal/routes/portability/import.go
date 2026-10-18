package portability

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/validation"
	"github.com/eventrouter/router/internal/store"
	"github.com/eventrouter/router/pkg/config"
)

// ImportMode tells what to do with a document whose name already exists in the team.
type ImportMode string

const (
	ImportCreate ImportMode = "create"
	ImportUpsert ImportMode = "upsert"
)

// ImportAction is the per-document outcome.
type ImportAction string

const (
	ActionCreated       ImportAction = "created"
	ActionDraftUpdated  ImportAction = "draft_updated"
	ActionSkippedExists ImportAction = "skipped_exists"
	ActionInvalid       ImportAction = "invalid"
	ActionFailed        ImportAction = "failed"
)

// Import issue codes of documents that cannot be imported.
const (
	IssueDocumentInvalid config.IssueCode = "document_invalid"
	IssueNameRequired    config.IssueCode = "name_required"
	IssueImportFailed    config.IssueCode = "import_failed"
)

// NamePointer addresses the document name.
var NamePointer = config.PointerRoot.Key(string(MemberName))

// ImportRequest is one import payload with its options.
type ImportRequest struct {
	Payload []byte
	Format  routes.DocumentFormat
	Mode    ImportMode
	DryRun  bool
	TeamID  routes.TeamID
}

// ImportResult is the outcome of one document.
type ImportResult struct {
	Index   int             `json:"index"`
	Name    string          `json:"name"`
	RouteID *routes.RouteID `json:"routeId,omitempty"`
	Action  ImportAction    `json:"action"`
	Issues  config.Issues   `json:"issues"`
}

// ImportResponse is the 200 body of POST /routes/import.
type ImportResponse struct {
	DryRun  bool           `json:"dryRun"`
	Results []ImportResult `json:"results"`
}

// Import decodes the payload and creates or updates one draft per document (never deploys).
func (m *PortabilityManager) Import(ctx context.Context, req ImportRequest) (ImportResponse, error) {
	if req.Mode == "" {
		req.Mode = ImportCreate
	}
	if req.Mode != ImportCreate && req.Mode != ImportUpsert {
		return ImportResponse{}, ErrBadImportMode
	}
	docs, err := m.decode(req)
	if err != nil {
		return ImportResponse{}, err
	}
	team, err := m.team(ctx, req.TeamID)
	if err != nil {
		return ImportResponse{}, err
	}
	out := ImportResponse{DryRun: req.DryRun, Results: make([]ImportResult, 0, len(docs))}
	run := importRun{m: m, team: team, req: req}
	for i, raw := range docs {
		out.Results = append(out.Results, run.one(ctx, i, raw))
	}
	return out, m.recordSummary(ctx, team, out)
}

// decode splits the payload into documents within the limits.
func (m *PortabilityManager) decode(req ImportRequest) ([]json.RawMessage, error) {
	codec, err := m.d.Codecs.Codec(req.Format)
	if err != nil {
		return nil, err
	}
	docs, err := codec.Decode(req.Payload)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, ErrNoDocuments
	}
	if len(docs) > int(MaxDocuments) {
		return nil, ErrTooManyDocuments
	}
	return docs, nil
}

// team is the requested team when writable, else the caller's first writable team.
func (m *PortabilityManager) team(ctx context.Context, requested routes.TeamID) (routes.TeamID, error) {
	actor := routes.ActorResolver{}.From(ctx)
	if requested == "" {
		team, ok := actor.DefaultTeam()
		if !ok {
			return "", routes.ErrNoWritableTeam
		}
		requested = team
	}
	if !m.d.Routes.Guard.CanWrite(actor, requested) {
		return "", routes.ErrForbiddenTeam
	}
	return requested, nil
}

// recordSummary audits an import that wrote nothing (dry run, all skipped or invalid) as one request entry.
func (m *PortabilityManager) recordSummary(ctx context.Context, team routes.TeamID, out ImportResponse) error {
	for _, r := range out.Results {
		if !out.DryRun && (r.Action == ActionCreated || r.Action == ActionDraftUpdated) {
			return nil
		}
	}
	return m.d.Auditor.RecordIfScoped(ctx, routes.AuditRecord{Action: routes.ActionRouteImported, Object: audit.ObjectRequest,
		ObjectID: string(ImportObjectID), Team: team, After: out})
}

// ImportObjectID is the audit object id of import requests that wrote no route.
type AuditObjectID string

const ImportObjectID AuditObjectID = "routes/import"

// importRun imports the documents of one request.
type importRun struct {
	m    *PortabilityManager
	team routes.TeamID
	req  ImportRequest
}

// one resolves, validates, classifies and (unless dry run) writes one document.
func (r importRun) one(ctx context.Context, index int, raw json.RawMessage) ImportResult {
	res := ImportResult{Index: index, Issues: config.Issues{}}
	doc, existing, err := r.prepare(ctx, raw, &res)
	if err != nil {
		return r.failed(res, err)
	}
	if res.Action == ActionInvalid {
		return res
	}
	res.Action = r.classify(existing)
	if existing != nil {
		id := existing.ID
		res.RouteID = &id
	}
	if r.req.DryRun || res.Action == ActionSkippedExists {
		return res
	}
	written, err := r.write(ctx, res.Action, existing, res.Name, doc)
	if err != nil {
		return r.failed(res, err)
	}
	res.RouteID = &written.ID
	return res
}

// prepare parses the document, resolves connections, validates it and finds a same-named route.
func (r importRun) prepare(ctx context.Context, raw json.RawMessage, res *ImportResult) (json.RawMessage, *routes.Route, error) {
	tree, err := ParseTree(raw)
	if err != nil {
		return r.invalid(res, config.NewError(IssueDocumentInvalid, config.PointerRoot, "This document is not a route object: %v", err))
	}
	res.Name = tree.Name()
	if res.Name == "" {
		return r.invalid(res, config.NewError(IssueNameRequired, NamePointer, "Give the route a name (the document's name member)."))
	}
	unresolved, err := r.m.d.Resolver.ToIDs(ctx, string(r.team), tree)
	if err != nil || len(unresolved) > 0 {
		res.Issues = append(res.Issues, unresolved...)
		res.Action = ActionInvalid
		return nil, nil, err
	}
	doc, err := tree.JSON()
	if err != nil {
		return nil, nil, err
	}
	existing, err := r.existing(ctx, res.Name)
	if err != nil {
		return nil, nil, err
	}
	res.Issues = append(res.Issues, r.validate(ctx, existing, res.Name, doc)...)
	return doc, existing, nil
}

func (importRun) invalid(res *ImportResult, issue config.Issue) (json.RawMessage, *routes.Route, error) {
	res.Issues = append(res.Issues, issue)
	res.Action = ActionInvalid
	return nil, nil, nil
}

// existing finds the team's route with name (nil when there is none).
func (r importRun) existing(ctx context.Context, name string) (*routes.Route, error) {
	route, err := r.m.d.Repos.Routes.FindByName(ctx, r.team, name)
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &route, nil
}

// validate runs the import mode; issues never block the draft (like Save draft).
func (r importRun) validate(ctx context.Context, existing *routes.Route, name string, doc json.RawMessage) config.Issues {
	req := validation.Request{Mode: validation.ModeImport, TeamID: string(r.team), RouteName: name, Raw: doc}
	if existing != nil {
		req.RouteID = string(existing.ID)
	}
	return r.m.d.Validator.Run(ctx, req).Issues
}

// classify picks created / draft_updated / skipped_exists from the mode.
func (r importRun) classify(existing *routes.Route) ImportAction {
	switch {
	case existing == nil:
		return ActionCreated
	case r.req.Mode == ImportUpsert:
		return ActionDraftUpdated
	default:
		return ActionSkippedExists
	}
}

// write creates the route or replaces the existing draft, then audits the import.
func (r importRun) write(ctx context.Context, action ImportAction, existing *routes.Route, name string, doc json.RawMessage) (routes.RouteDetail, error) {
	var d routes.RouteDetail
	var err error
	if action == ActionCreated {
		d, err = r.m.d.Routes.Create(ctx, routes.NewRoute{Name: name, TeamID: r.team, Draft: doc})
	} else {
		d, err = r.m.d.Routes.Update(ctx, existing.ID, routes.RouteChange{Draft: doc}, existing.Revision)
	}
	if err != nil {
		return d, err
	}
	return d, r.m.d.Auditor.Record(ctx, routes.AuditRecord{Action: routes.ActionRouteImported, Object: audit.ObjectRoute,
		ObjectID: string(d.ID), Team: r.team, After: ImportAuditView{Name: name, Action: action}})
}

// failed turns a write error into the document's result.
func (r importRun) failed(res ImportResult, err error) ImportResult {
	d := r.m.d.Catalog.Resolve(err)
	res.Action = ActionFailed
	res.Issues = append(res.Issues, config.NewError(IssueImportFailed, config.Pointer(d.Path), "%s", r.message(d.Detail, d.Title)))
	return res
}

func (importRun) message(detail, title string) string {
	if detail != "" {
		return detail
	}
	return title
}

// ImportAuditView is the audited summary of one imported document.
type ImportAuditView struct {
	Name   string       `json:"name"`
	Action ImportAction `json:"action"`
}
