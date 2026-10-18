package routestest

import (
	"context"
	"sort"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/routes"
	"github.com/eventrouter/router/internal/store"
)

// VersionRepo is the in-memory routes.VersionRepository.
type VersionRepo struct {
	db *DB
}

func (r *VersionRepo) Create(_ context.Context, v routes.RouteVersion) (routes.RouteVersion, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	max := 0
	for _, existing := range r.db.state.Versions {
		if existing.RouteID == v.RouteID && existing.No > max {
			max = existing.No
		}
	}
	v.ID, v.No, v.CreatedAt = routes.VersionID(uuid.NewString()), max+1, r.db.now()
	if v.BasedOn != nil {
		no := r.db.state.Versions[*v.BasedOn].No
		v.BasedOnNo = &no
	}
	r.db.state.Versions[v.ID] = v
	return v, nil
}

func (r *VersionRepo) List(_ context.Context, id routes.RouteID, page routes.PageRequest) ([]routes.RouteVersion, *store.Cursor, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	out := []routes.RouteVersion{}
	for _, v := range r.db.state.Versions {
		if v.RouteID == id {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].No > out[j].No })
	out = Pager[routes.RouteVersion]{Key: func(v routes.RouteVersion) (string, string) { return IntKey(v.No), string(v.ID) }}.After(out, page)
	kept, next := store.PageSlicer[routes.RouteVersion]{Limit: page.Limit, KeyOf: func(v routes.RouteVersion) store.Cursor {
		return store.Cursor{SortKey: []string{IntKey(v.No)}, ID: string(v.ID)}
	}}.Slice(out)
	return kept, next, nil
}

func (r *VersionRepo) GetByNo(_ context.Context, id routes.RouteID, no int) (routes.RouteVersion, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	for _, v := range r.db.state.Versions {
		if v.RouteID == id && v.No == no {
			return v, nil
		}
	}
	return routes.RouteVersion{}, store.ErrNotFound
}

func (r *VersionRepo) GetByID(_ context.Context, id routes.VersionID) (routes.RouteVersion, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	v, ok := r.db.state.Versions[id]
	if !ok {
		return routes.RouteVersion{}, store.ErrNotFound
	}
	return v, nil
}

// DeploymentRepo is the in-memory routes.DeploymentRepository.
type DeploymentRepo struct {
	db *DB
}

func (r *DeploymentRepo) Create(_ context.Context, d routes.Deployment) (routes.Deployment, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	d.ID, d.RequestedAt = routes.DeploymentID(uuid.NewString()), r.db.now()
	if d.Status == routes.DeployRollingOut {
		t := d.RequestedAt
		d.StartedAt = &t
	}
	r.db.state.Deployments[d.ID] = d
	return d, nil
}

func (r *DeploymentRepo) Get(_ context.Context, id routes.DeploymentID) (routes.Deployment, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	d, ok := r.db.state.Deployments[id]
	if !ok {
		return routes.Deployment{}, store.ErrNotFound
	}
	return d, nil
}

func (r *DeploymentRepo) SupersedeOpen(_ context.Context, id routes.RouteID, scope routes.SupersedeScope) error {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	for k, d := range r.db.state.Deployments {
		if d.RouteID == id && scope.Covers(d) {
			d.Status = routes.DeploySuperseded
			r.db.state.Deployments[k] = d
		}
	}
	return nil
}

func (r *DeploymentRepo) Transition(_ context.Context, c routes.StatusChange) (routes.Deployment, error) {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	d, ok := r.db.state.Deployments[c.ID]
	if !ok {
		return routes.Deployment{}, store.ErrNotFound
	}
	if d.Status != c.From {
		return routes.Deployment{}, routes.ErrTransitionNotAllowed
	}
	d.Status = c.To
	if c.Reason != "" {
		d.FailureReason = c.Reason
	}
	if c.ApprovedBy != nil {
		d.ApprovedBy = c.ApprovedBy
	}
	r.db.state.Deployments[c.ID] = d
	return d, nil
}

func (r *DeploymentRepo) Latest(_ context.Context, id routes.RouteID) (*routes.Deployment, error) {
	list := r.sorted(id)
	if len(list) == 0 {
		return nil, nil
	}
	return &list[0], nil
}

func (r *DeploymentRepo) sorted(id routes.RouteID) []routes.Deployment {
	r.db.mu.Lock()
	defer r.db.mu.Unlock()
	out := []routes.Deployment{}
	for _, d := range r.db.state.Deployments {
		if d.RouteID == id {
			out = append(out, d)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RequestedAt.After(out[j].RequestedAt) })
	return out
}

func (r *DeploymentRepo) List(_ context.Context, id routes.RouteID, page routes.PageRequest) ([]routes.Deployment, *store.Cursor, error) {
	kept, next := store.PageSlicer[routes.Deployment]{Limit: page.Limit, KeyOf: func(d routes.Deployment) store.Cursor {
		return store.Cursor{SortKey: []string{d.RequestedAt.String()}, ID: string(d.ID)}
	}}.Slice(r.sorted(id))
	return kept, next, nil
}

func (r *DeploymentRepo) StatusByVersion(ctx context.Context, ids []routes.VersionID) (map[routes.VersionID]routes.DeploymentStatus, error) {
	out := map[routes.VersionID]routes.DeploymentStatus{}
	r.db.mu.Lock()
	all := []routes.Deployment{}
	for _, d := range r.db.state.Deployments {
		all = append(all, d)
	}
	r.db.mu.Unlock()
	sort.Slice(all, func(i, j int) bool { return all[i].RequestedAt.Before(all[j].RequestedAt) })
	wanted := map[routes.VersionID]bool{}
	for _, id := range ids {
		wanted[id] = true
	}
	for _, d := range all {
		if wanted[d.VersionID] {
			out[d.VersionID] = d.Status
		}
	}
	return out, nil
}
