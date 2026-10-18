// Package approvalstest provides in-memory approvals collaborators (store, access, teams, activator,
// connection directory) so the approval state machine can be tested without Postgres.
package approvalstest

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/approvals"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// Store is an in-memory approvals.Store.
type Store struct {
	mu        sync.Mutex
	Items     map[uuid.UUID]*approvals.StoredApproval
	Notes     map[uuid.UUID][]approvals.Comment
	Approvals []uuid.UUID
	Active    map[uuid.UUID]*approvals.VersionConfig
}

// NewStore returns an empty store.
func NewStore() *Store {
	return &Store{Items: map[uuid.UUID]*approvals.StoredApproval{}, Notes: map[uuid.UUID][]approvals.Comment{},
		Active: map[uuid.UUID]*approvals.VersionConfig{}}
}

// AddPending stores a pending request of requester on team.
func (s *Store) AddPending(team rbac.TeamID, requester uuid.UUID) uuid.UUID {
	id := uuid.New()
	s.Items[id] = &approvals.StoredApproval{Approval: approvals.Approval{ID: id, RouteID: uuid.New(), RouteName: "orders", TeamID: team,
		VersionNo: 2, Status: approvals.StatusPending, RequestedBy: &approvals.Person{ID: requester, Name: "req"},
		RequestedAt: time.Now().Add(-time.Minute), RowVersion: 1, Destinations: []approvals.Destination{}}, Config: []byte(`{}`)}
	return id
}

func (s *Store) List(_ context.Context, f approvals.ListFilter, _ approvals.Page) ([]approvals.StoredApproval, *store.Cursor, error) {
	out := []approvals.StoredApproval{}
	for _, a := range s.Items {
		if f.Status == nil || *f.Status == a.Status {
			out = append(out, *a)
		}
	}
	return out, nil, nil
}

func (s *Store) Get(_ context.Context, id uuid.UUID) (approvals.StoredApproval, error) {
	a, ok := s.Items[id]
	if !ok {
		return approvals.StoredApproval{}, store.ErrNotFound
	}
	return *a, nil
}

func (s *Store) Lock(ctx context.Context, id uuid.UUID) (approvals.StoredApproval, error) {
	return s.Get(ctx, id)
}

func (s *Store) ActiveVersion(_ context.Context, route uuid.UUID) (*approvals.VersionConfig, error) {
	return s.Active[route], nil
}

func (s *Store) SetDecisionNote(_ context.Context, id uuid.UUID, note string) error {
	s.Items[id].DecisionNote = note
	return nil
}

func (s *Store) Reject(_ context.Context, id, by uuid.UUID, note string) error {
	return s.move(id, approvals.StatusRejected, func(a *approvals.StoredApproval) {
		now := time.Now()
		a.DecidedBy, a.DecidedAt, a.DecisionNote = &approvals.Person{ID: by}, &now, note
	})
}

func (s *Store) Cancel(_ context.Context, id uuid.UUID, reason approvals.CancelReason) error {
	return s.move(id, approvals.StatusCancelled, func(a *approvals.StoredApproval) { a.CancelReason = reason })
}

// Approve is what the fake activator applies.
func (s *Store) Approve(id, by uuid.UUID) error {
	return s.move(id, approvals.StatusRollingOut, func(a *approvals.StoredApproval) {
		now := time.Now()
		a.DecidedBy, a.DecidedAt = &approvals.Person{ID: by}, &now
	})
}

func (s *Store) move(id uuid.UUID, to approvals.ApprovalStatus, apply func(*approvals.StoredApproval)) error {
	a, ok := s.Items[id]
	if !ok || a.Status != approvals.StatusPending {
		return approvals.ErrNotPending
	}
	a.Status, a.RowVersion = to, a.RowVersion+1
	apply(a)
	return nil
}

func (s *Store) CancelOtherPending(_ context.Context, route, except uuid.UUID) ([]uuid.UUID, error) {
	out := []uuid.UUID{}
	for id, a := range s.Items {
		if a.RouteID == route && id != except && a.Status == approvals.StatusPending {
			a.Status, a.CancelReason = approvals.StatusCancelled, approvals.CancelSuperseded
			out = append(out, id)
		}
	}
	return out, nil
}

func (s *Store) Comments(_ context.Context, id uuid.UUID) ([]approvals.Comment, error) {
	return append([]approvals.Comment{}, s.Notes[id]...), nil
}

func (s *Store) AddComment(_ context.Context, id, author uuid.UUID, body string) (approvals.Comment, error) {
	c := approvals.Comment{ID: uuid.New(), ApprovalID: id, Author: &approvals.Person{ID: author}, Body: body, CreatedAt: time.Now()}
	s.Notes[id] = append(s.Notes[id], c)
	return c, nil
}

func (s *Store) Approvers(_ context.Context, _ rbac.TeamID, excluding uuid.UUID) ([]uuid.UUID, error) {
	out := []uuid.UUID{}
	for _, id := range s.Approvals {
		if id != excluding {
			out = append(out, id)
		}
	}
	return out, nil
}

func (s *Store) Commenters(_ context.Context, id uuid.UUID) ([]uuid.UUID, error) {
	out := []uuid.UUID{}
	for _, c := range s.Notes[id] {
		out = append(out, c.Author.ID)
	}
	return out, nil
}

func (s *Store) PendingByTeam(context.Context) (map[rbac.TeamID]int, error) {
	out := map[rbac.TeamID]int{}
	for _, a := range s.Items {
		if a.Status == approvals.StatusPending {
			out[a.TeamID]++
		}
	}
	return out, nil
}

// Activator applies approvals to the store; Invalid fails re-validation, Err fails activation.
type Activator struct {
	Store   *Store
	Invalid error
	Err     error
}

// Revalidate implements approvals.Activator.
func (a *Activator) Revalidate(context.Context, uuid.UUID) error {
	return a.Invalid
}

// Activate implements approvals.Activator.
func (a *Activator) Activate(_ context.Context, id, approver uuid.UUID) error {
	if a.Err != nil {
		return a.Err
	}
	return a.Store.Approve(id, approver)
}

// Teams is an in-memory team policy reader.
type Teams map[rbac.TeamID]rbac.Team

// Get implements rbac.TeamPolicyReader.
func (t Teams) Get(_ context.Context, id rbac.TeamID) (rbac.Team, error) {
	team, ok := t[id]
	if !ok {
		return rbac.Team{}, store.ErrNotFound
	}
	return team, nil
}

// Connections is an in-memory connection directory.
type Connections map[uuid.UUID]rbac.ConnectionInfo

// Find implements rbac.ConnectionDirectory.
func (c Connections) Find(_ context.Context, ids []uuid.UUID) (map[uuid.UUID]rbac.ConnectionInfo, error) {
	out := map[uuid.UUID]rbac.ConnectionInfo{}
	for _, id := range ids {
		if info, ok := c[id]; ok {
			out[id] = info
		}
	}
	return out, nil
}

// As returns ctx carrying a user principal.
func As(ctx context.Context, user uuid.UUID) context.Context {
	return auth.WithPrincipal(ctx, auth.Principal{Kind: auth.PrincipalUser, UserID: user})
}

// TeamOf implements rbac.TeamOwner for approval ids.
func (s *Store) TeamOf(_ context.Context, id string) (string, error) {
	for key, a := range s.Items {
		if key.String() == id {
			return a.TeamID.String(), nil
		}
	}
	return "", store.ErrNotFound
}
