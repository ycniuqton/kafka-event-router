package approvals

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/pkg/config"
)

// ListWhere renders a ListFilter into predicates with positional arguments.
type ListWhere struct{}

// Build returns the predicates and their arguments.
func (ListWhere) Build(f ListFilter) ([]string, []any) {
	b := &predicates{}
	if f.Status != nil {
		b.add("d.status::text = $%d", string(*f.Status))
	}
	if f.TeamID != nil {
		b.add("r.team_id = $%d", *f.TeamID)
	}
	if f.RouteID != nil {
		b.add("d.route_id = $%d", *f.RouteID)
	}
	if f.Mine {
		b.add("d.requested_by = $%d", f.Caller)
	}
	return b.where, b.args
}

// predicates accumulates numbered predicates.
type predicates struct {
	where []string
	args  []any
}

func (p *predicates) add(format string, arg any) {
	p.args = append(p.args, arg)
	p.where = append(p.where, fmt.Sprintf(format, len(p.args)))
}

// DestinationReader derives a version's branch destinations (connection, name, env tag) from its config.
type DestinationReader struct {
	Connections rbac.ConnectionDirectory
}

// IDs returns the distinct destination connection ids of a raw route config (unparsable configs have none).
func (DestinationReader) IDs(raw []byte) []uuid.UUID {
	doc, err := config.Parse(raw)
	if err != nil {
		return nil
	}
	cfg, err := doc.Config()
	if err != nil {
		return nil
	}
	seen := map[uuid.UUID]bool{}
	out := []uuid.UUID{}
	for _, b := range cfg.Branches {
		id, err := uuid.Parse(b.Destination.ConnectionID)
		if err == nil && !seen[id] {
			seen[id], out = true, append(out, id)
		}
	}
	return out
}

// Fill sets Destinations on every approval with one directory lookup.
func (r DestinationReader) Fill(ctx context.Context, items []StoredApproval) error {
	ids := []uuid.UUID{}
	for _, a := range items {
		ids = append(ids, r.IDs(a.Config)...)
	}
	found, err := r.Connections.Find(ctx, ids)
	if err != nil {
		return err
	}
	for i := range items {
		items[i].Destinations = r.of(items[i].Config, found)
	}
	return nil
}

// Resources returns the destinations of raw as authorization resources.
func (r DestinationReader) Resources(ctx context.Context, raw []byte) ([]rbac.Resource, error) {
	found, err := r.Connections.Find(ctx, r.IDs(raw))
	if err != nil {
		return nil, err
	}
	out := []rbac.Resource{}
	for _, c := range found {
		out = append(out, c.Resource())
	}
	return out, nil
}

func (r DestinationReader) of(raw []byte, found map[uuid.UUID]rbac.ConnectionInfo) []Destination {
	out := []Destination{}
	for _, id := range r.IDs(raw) {
		c := found[id]
		out = append(out, Destination{ConnectionID: id.String(), Name: c.Name, EnvTag: c.EnvTag})
	}
	return out
}
