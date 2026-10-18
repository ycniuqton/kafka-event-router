// Package bulk applies one action to many routes (POST /routes/bulk); each id is handled on its
// own and reports its own result, so one failure never stops the others.
package bulk

import (
	"context"
	"strings"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/routes/portability"
)

// Args are the action arguments of a bulk request.
type Args struct {
	Tag    string
	Format routes.DocumentFormat
}

// Handler applies one bulk action to one route.
type Handler interface {
	Action() routes.BulkAction
	Apply(ctx context.Context, id routes.RouteID, args Args) error
}

// Finisher is implemented by handlers that produce one artifact for all successful ids (export).
type Finisher interface {
	Finish(ctx context.Context, ids []routes.RouteID, args Args) (*portability.Artifact, error)
}

// Request is the decoded body of POST /routes/bulk.
type Request struct {
	Action routes.BulkAction
	IDs    []routes.RouteID
	Args   Args
}

// ItemResult is the outcome for one id.
type ItemResult struct {
	ID      routes.RouteID   `json:"id"`
	OK      bool             `json:"ok"`
	Problem *problem.Details `json:"problem,omitempty"`
}

// Response is the 200 body.
type Response struct {
	Results []ItemResult          `json:"results"`
	Export  *portability.Artifact `json:"export,omitempty"`
}

// BulkActionManager dispatches to the handler registered for the action.
type BulkActionManager struct {
	handlers map[routes.BulkAction]Handler
	catalog  *problem.Catalog
}

// NewBulkActionManager registers handlers; catalog renders per-id problems.
func NewBulkActionManager(catalog *problem.Catalog, hs ...Handler) *BulkActionManager {
	m := &BulkActionManager{handlers: map[routes.BulkAction]Handler{}, catalog: catalog}
	for _, h := range hs {
		m.handlers[h.Action()] = h
	}
	return m
}

// Run validates the request and applies the handler to every id in order.
func (m *BulkActionManager) Run(ctx context.Context, req Request) (Response, error) {
	h, ok := m.handlers[req.Action]
	if !ok {
		return Response{}, routes.ErrUnknownBulkAction
	}
	if err := m.check(req); err != nil {
		return Response{}, err
	}
	out := Response{Results: make([]ItemResult, 0, len(req.IDs))}
	for _, id := range req.IDs {
		out.Results = append(out.Results, m.one(ctx, h, id, req.Args))
	}
	return m.finish(ctx, h, req.Args, out)
}

// finish lets a Finisher build its artifact over the ids that succeeded.
func (m *BulkActionManager) finish(ctx context.Context, h Handler, args Args, out Response) (Response, error) {
	f, ok := h.(Finisher)
	if !ok {
		return out, nil
	}
	ids := []routes.RouteID{}
	for _, r := range out.Results {
		if r.OK {
			ids = append(ids, r.ID)
		}
	}
	if len(ids) == 0 {
		return out, nil
	}
	artifact, err := f.Finish(ctx, ids, args)
	out.Export = artifact
	return out, err
}

func (m *BulkActionManager) check(req Request) error {
	if len(req.IDs) > int(routes.MaxBulkIDs) {
		return routes.ErrTooManyIDs
	}
	v := api.NewValidationError()
	if len(req.IDs) == 0 {
		v.Add(string(routes.PathIDs), string(routes.CodeRequired), "Select at least one route.")
	}
	if req.Action == routes.BulkAddTag && strings.TrimSpace(req.Args.Tag) == "" {
		v.Add(string(routes.PathTag), string(routes.CodeRequired), "Enter the tag to add.")
	}
	return v.OrNil()
}

func (m *BulkActionManager) one(ctx context.Context, h Handler, id routes.RouteID, args Args) ItemResult {
	if err := h.Apply(ctx, id, args); err != nil {
		d := m.catalog.Resolve(err)
		return ItemResult{ID: id, Problem: &d}
	}
	return ItemResult{ID: id, OK: true}
}
