package routes

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"time"

	"github.com/eventrouter/router/internal/routes/diff"
	"github.com/eventrouter/router/internal/store"
)

// VersionSummary is one row of GET /routes/{id}/versions.
type VersionSummary struct {
	No         int               `json:"no"`
	Origin     VersionOrigin     `json:"origin"`
	BasedOn    *int              `json:"basedOn,omitempty"`
	Author     *UserRef          `json:"author"`
	ChangeNote string            `json:"changeNote"`
	CreatedAt  time.Time         `json:"createdAt"`
	ConfigHash string            `json:"configHash"`
	Active     bool              `json:"active"`
	Deployment *DeploymentStatus `json:"deploymentStatus"`
}

// VersionManager lists and reads versions and diffs any two of them (or active / draft).
type VersionManager struct {
	*Core
	Differ *diff.RouteDiffer
}

// NewVersionManager builds the manager with the default differ.
func NewVersionManager(core *Core) *VersionManager {
	return &VersionManager{Core: core, Differ: diff.DefaultRouteDiffer()}
}

// List returns one page of versions, newest first, with the latest deployment status of each.
func (m *VersionManager) List(ctx context.Context, id RouteID, page PageRequest) ([]VersionSummary, *store.Cursor, error) {
	route, err := m.load(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	versions, next, err := m.Repos.Versions.List(ctx, id, page)
	if err != nil {
		return nil, nil, err
	}
	ids := make([]VersionID, len(versions))
	for i, v := range versions {
		ids[i] = v.ID
	}
	statuses, err := m.Repos.Deployments.StatusByVersion(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	out := make([]VersionSummary, len(versions))
	for i, v := range versions {
		out[i] = m.summary(route, v, statuses)
	}
	return out, next, nil
}

func (VersionManager) summary(route Route, v RouteVersion, statuses map[VersionID]DeploymentStatus) VersionSummary {
	s := VersionSummary{No: v.No, Origin: v.Origin, BasedOn: v.BasedOnNo, Author: v.Author, ChangeNote: v.ChangeNote,
		CreatedAt: v.CreatedAt, ConfigHash: v.ConfigHash, Active: route.ActiveVersionID != nil && *route.ActiveVersionID == v.ID}
	if st, ok := statuses[v.ID]; ok {
		s.Deployment = &st
	}
	return s
}

// Get returns one version with its full config.
func (m *VersionManager) Get(ctx context.Context, id RouteID, no int) (RouteVersion, error) {
	if _, err := m.load(ctx, id); err != nil {
		return RouteVersion{}, err
	}
	v, err := m.Repos.Versions.GetByNo(ctx, id, no)
	if errors.Is(err, store.ErrNotFound) {
		return RouteVersion{}, ErrVersionNotFound
	}
	return v, err
}

// Diff compares two references: a version number, "active" or "draft"; a missing side is empty.
func (m *VersionManager) Diff(ctx context.Context, id RouteID, refA, refB string) (diff.RouteDiff, error) {
	route, err := m.load(ctx, id)
	if err != nil {
		return diff.RouteDiff{}, err
	}
	a, err := m.resolve(ctx, route, refA)
	if err != nil {
		return diff.RouteDiff{}, err
	}
	b, err := m.resolve(ctx, route, refB)
	if err != nil {
		return diff.RouteDiff{}, err
	}
	return m.Differ.Diff(refA, refB, ConfigOf(a), ConfigOf(b)), nil
}

// Config returns the route and the config ref points at; an empty ref is the live version, else the draft.
func (m *VersionManager) Config(ctx context.Context, id RouteID, ref string) (Route, json.RawMessage, error) {
	route, err := m.load(ctx, id)
	if err != nil {
		return Route{}, nil, err
	}
	if ref == "" {
		ref = string(RefActive)
		if route.ActiveVersionID == nil {
			ref = string(RefDraft)
		}
	}
	raw, err := m.resolve(ctx, route, ref)
	return route, raw, err
}

// resolve returns the config a reference points at (nil when that side does not exist).
func (m *VersionManager) resolve(ctx context.Context, route Route, ref string) (json.RawMessage, error) {
	switch VersionRefKind(ref) {
	case RefDraft:
		return route.Draft, nil
	case RefActive:
		if route.ActiveVersionID == nil {
			return nil, nil
		}
		v, err := m.Repos.Versions.GetByID(ctx, *route.ActiveVersionID)
		return v.Config, err
	}
	no, err := strconv.Atoi(ref)
	if err != nil || no < 1 {
		return nil, ErrBadVersionRef
	}
	v, err := m.Repos.Versions.GetByNo(ctx, route.ID, no)
	if errors.Is(err, store.ErrNotFound) {
		return nil, ErrVersionNotFound
	}
	return v.Config, err
}

// Deployments lists a route's deployments, newest first.
func (m *VersionManager) Deployments(ctx context.Context, id RouteID, page PageRequest) ([]Deployment, *store.Cursor, error) {
	if _, err := m.load(ctx, id); err != nil {
		return nil, nil, err
	}
	return m.Repos.Deployments.List(ctx, id, page)
}
