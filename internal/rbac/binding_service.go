package rbac

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/store"
)

// SetBinding is the command behind PUT /role-bindings/{userId}/{teamId}.
type SetBinding struct {
	UserID  uuid.UUID
	TeamID  TeamID
	Role    Role
	IfMatch *store.RowVersion
}

// ErrBindingPrecondition is returned when an existing binding is changed without If-Match (428).
var ErrBindingPrecondition = problem.NewError(problem.KindPreconditionRequired,
	"This change needs the version you edited. Reload the member list and retry.").At("/If-Match")

// BindingService changes role bindings with the last-admin guard, phase-assignable roles and audit (flow 5.2).
type BindingService struct {
	tx         store.TxManager
	bindings   BindingStore
	subjects   SubjectLoader
	guard      *AdminGuard
	assignable RoleSet
	trail      *AuditTrail
}

// BindingServiceDeps groups the collaborators of a BindingService.
type BindingServiceDeps struct {
	Tx         store.TxManager
	Bindings   BindingStore
	Subjects   SubjectLoader
	Guard      *AdminGuard
	Assignable RoleSet
	Trail      *AuditTrail
}

// NewBindingService builds the service.
func NewBindingService(d BindingServiceDeps) *BindingService {
	return &BindingService{tx: d.Tx, bindings: d.Bindings, subjects: d.Subjects, guard: d.Guard, assignable: d.Assignable, trail: d.Trail}
}

// List returns bindings matching f.
func (s *BindingService) List(ctx context.Context, f BindingFilter) ([]RoleBinding, error) {
	return s.bindings.List(ctx, f)
}

// AssignableRoles returns the roles assignable in the current phase.
func (s *BindingService) AssignableRoles() []Role {
	return s.assignable.Ordered()
}

// Set creates or changes a binding; changing an existing one requires its current version.
func (s *BindingService) Set(ctx context.Context, cmd SetBinding) (RoleBinding, error) {
	if !s.assignable.Has(cmd.Role) {
		return RoleBinding{}, ErrRoleNotAssignable
	}
	var out RoleBinding
	err := s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.guard.Lock(ctx); err != nil {
			return err
		}
		before, err := s.bindings.GetForUpdate(ctx, cmd.UserID, cmd.TeamID)
		if errors.Is(err, store.ErrNotFound) {
			out, err = s.create(ctx, cmd)
			return err
		}
		if err != nil {
			return err
		}
		out, err = s.change(ctx, before, cmd)
		return err
	})
	return out, err
}

// Remove deletes a binding (requires its current version) unless it is the last admin.
func (s *BindingService) Remove(ctx context.Context, userID uuid.UUID, team TeamID, ifMatch *store.RowVersion) error {
	return s.tx.WithinTx(ctx, func(ctx context.Context) error {
		if err := s.guard.Lock(ctx); err != nil {
			return err
		}
		before, err := s.bindings.GetForUpdate(ctx, userID, team)
		if err != nil {
			return err
		}
		if err := s.checkLoss(ctx, before, ifMatch); err != nil {
			return err
		}
		if err := s.bindings.Delete(ctx, userID, team); err != nil {
			return err
		}
		return s.record(ctx, audit.ActionRoleBindingRemoved, &before, nil)
	})
}

func (s *BindingService) create(ctx context.Context, cmd SetBinding) (RoleBinding, error) {
	b, err := s.bindings.Insert(ctx, cmd.UserID, cmd.TeamID, cmd.Role)
	if err != nil {
		return RoleBinding{}, err
	}
	return b, s.record(ctx, audit.ActionRoleBindingSet, nil, &b)
}

func (s *BindingService) change(ctx context.Context, before RoleBinding, cmd SetBinding) (RoleBinding, error) {
	if before.Role == cmd.Role {
		return before, nil
	}
	if err := s.checkLoss(ctx, before, cmd.IfMatch); err != nil {
		return RoleBinding{}, err
	}
	after, err := s.bindings.UpdateRole(ctx, cmd.UserID, cmd.TeamID, cmd.Role)
	if err != nil {
		return RoleBinding{}, err
	}
	return after, s.record(ctx, audit.ActionRoleBindingSet, &before, &after)
}

// checkLoss verifies the precondition and that the change keeps an administrator.
func (s *BindingService) checkLoss(ctx context.Context, before RoleBinding, ifMatch *store.RowVersion) error {
	if ifMatch == nil {
		return ErrBindingPrecondition
	}
	if *ifMatch != before.RowVersion {
		return &store.PreconditionFailedError{Current: before.RowVersion}
	}
	subject, err := s.subjects.Load(ctx, before.UserID)
	if err != nil {
		return err
	}
	return s.guard.CheckBindingLoss(ctx, before, subject.WorkspaceAdmin && !subject.Disabled)
}

func (s *BindingService) record(ctx context.Context, action audit.Action, before, after *RoleBinding) error {
	ref := after
	if ref == nil {
		ref = before
	}
	team := ref.TeamID
	return s.trail.Record(ctx, audit.Change{Action: action, Object: audit.ObjectRoleBinding,
		ObjectID: ref.UserID.String() + "/" + ref.TeamID.String(), TeamID: &team, Before: BindingAudit{}.Of(before), After: BindingAudit{}.Of(after)})
}

// BindingAudit renders bindings for audit diffs (nil → nil so creations and removals diff fully).
type BindingAudit struct{}

// Of returns the audited fields of b.
func (BindingAudit) Of(b *RoleBinding) any {
	if b == nil {
		return nil
	}
	return map[string]any{"userId": b.UserID, "teamId": b.TeamID, "role": b.Role}
}
