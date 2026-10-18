package approvals

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/platform/logging"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// Manager is the only entry point to approvals (routes calls Gate/Request; the HTTP API the rest).
type Manager interface {
	Gate(ctx context.Context, t DeployTarget) (GateResult, error)
	Request(ctx context.Context, in RequestInput) (Approval, error)
	Approve(ctx context.Context, id uuid.UUID, note string, ifMatch store.RowVersion) (Approval, error)
	Reject(ctx context.Context, id uuid.UUID, note string, ifMatch store.RowVersion) (Approval, error)
	Withdraw(ctx context.Context, id uuid.UUID) (Approval, error)
	Comment(ctx context.Context, id uuid.UUID, body string) (Comment, error)
	List(ctx context.Context, f ListFilter, page Page) ([]Approval, *store.Cursor, error)
	Get(ctx context.Context, id uuid.UUID) (ApprovalDetail, error)
}

// CommentLimit bounds comment bodies.
type CommentLimit int

const MaxCommentLength CommentLimit = 4000

// Deps groups the collaborators of the DefaultManager.
type Deps struct {
	Tx           store.TxManager
	Store        Store
	Access       rbac.Manager
	Teams        rbac.TeamPolicyReader
	Destinations DestinationReader
	Policy       ApprovalPolicy
	Activator    Activator
	Notifier     Notifier
	Trail        *rbac.AuditTrail
	Metrics      *Metrics
	Now          func() time.Time
	Log          *slog.Logger
}

// DefaultManager implements flows 5.4–5.6 (strict 4-eyes, race-safe decisions under FOR UPDATE).
type DefaultManager struct {
	d Deps
}

// NewManager builds the manager.
func NewManager(d Deps) *DefaultManager {
	if d.Now == nil {
		d.Now = time.Now
	}
	return &DefaultManager{d: d}
}

// Gate evaluates the configured policy for the caller on the route's team and destinations.
func (m *DefaultManager) Gate(ctx context.Context, t DeployTarget) (GateResult, error) {
	team, err := m.d.Teams.Get(ctx, t.TeamID)
	if err != nil {
		return GateResult{}, err
	}
	rights := RightsReader{Access: m.d.Access}.Of(ctx, t.RouteID, t.TeamID)
	res := m.d.Policy.Evaluate(ctx, rights, t, TeamPolicy{RequireProdApproval: team.RequireProdApproval})
	if res.Outcome != OutcomeRequireApproval {
		return res, nil
	}
	approvers, err := m.d.Store.Approvers(ctx, t.TeamID, auth.PrincipalFrom(ctx).UserID)
	if err != nil {
		return GateResult{}, err
	}
	res.Approvers = len(approvers)
	if res.Approvers == 0 {
		res.Reason = ReasonNoApprover
	}
	return res, nil
}

// Request registers a pending deployment created by routes (same tx): older pending requests of the route are
// cancelled as superseded, then approvers are notified.
func (m *DefaultManager) Request(ctx context.Context, in RequestInput) (Approval, error) {
	var out Approval
	err := m.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		cancelled, err := m.d.Store.CancelOtherPending(ctx, in.RouteID, in.DeploymentID)
		if err != nil {
			return err
		}
		if out, err = m.load(ctx, in.DeploymentID); err != nil {
			return err
		}
		for _, id := range cancelled {
			if err := m.record(ctx, audit.ActionDeploymentWithdrawn, out, id, CancelSuperseded); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return Approval{}, err
	}
	m.notify(ctx, EventRequested, out, "")
	return out, nil
}

// Approve re-validates the requested version, then activates it through routes in one transaction under
// FOR UPDATE (exactly one decision wins); the requester can never approve, admins included. The Activator
// (routes' lifecycle) records deployment.approved with the activated deployment.
func (m *DefaultManager) Approve(ctx context.Context, id uuid.UUID, note string, ifMatch store.RowVersion) (Approval, error) {
	actor := auth.PrincipalFrom(ctx).UserID
	if err := m.d.Activator.Revalidate(ctx, id); err != nil {
		return Approval{}, err
	}
	var before, out Approval
	err := m.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if before, err = m.decidable(ctx, id, ifMatch, actor); err != nil {
			return err
		}
		if err := m.d.Store.SetDecisionNote(ctx, id, strings.TrimSpace(note)); err != nil {
			return err
		}
		if err := m.d.Activator.Activate(ctx, id, actor); err != nil {
			return err
		}
		out, err = m.load(ctx, id)
		return err
	})
	if err != nil {
		return Approval{}, err
	}
	m.d.Metrics.Decided(before, StatusRollingOut, m.d.Now())
	m.notify(ctx, EventDecided, out, note)
	return out, nil
}

// Reject closes a pending request with a mandatory note; the draft stays editable.
func (m *DefaultManager) Reject(ctx context.Context, id uuid.UUID, note string, ifMatch store.RowVersion) (Approval, error) {
	note = strings.TrimSpace(note)
	if note == "" {
		return Approval{}, ErrNoteRequired
	}
	actor := auth.PrincipalFrom(ctx).UserID
	var before, out Approval
	err := m.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if before, err = m.decidable(ctx, id, ifMatch, uuid.Nil); err != nil {
			return err
		}
		if err := m.d.Store.Reject(ctx, id, actor, note); err != nil {
			return err
		}
		if out, err = m.load(ctx, id); err != nil {
			return err
		}
		return m.record(ctx, audit.ActionDeploymentRejected, out, id, "")
	})
	if err != nil {
		return Approval{}, err
	}
	m.d.Metrics.Decided(before, StatusRejected, m.d.Now())
	m.notify(ctx, EventDecided, out, note)
	return out, nil
}

// Withdraw cancels a pending request; only its requester or a workspace admin may.
func (m *DefaultManager) Withdraw(ctx context.Context, id uuid.UUID) (Approval, error) {
	var out Approval
	err := m.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		a, err := m.pending(ctx, id)
		if err != nil {
			return err
		}
		if err := m.mayWithdraw(ctx, a.Approval); err != nil {
			return err
		}
		if err := m.d.Store.Cancel(ctx, id, CancelWithdrawn); err != nil {
			return err
		}
		if out, err = m.load(ctx, id); err != nil {
			return err
		}
		return m.record(ctx, audit.ActionDeploymentWithdrawn, out, id, CancelWithdrawn)
	})
	if err != nil {
		return Approval{}, err
	}
	m.notify(ctx, EventWithdrawn, out, "")
	return out, nil
}

// Comment adds a review comment (1–4000 characters) by anyone who may read the route.
func (m *DefaultManager) Comment(ctx context.Context, id uuid.UUID, body string) (Comment, error) {
	body = strings.TrimSpace(body)
	if body == "" || utf8.RuneCountInString(body) > int(MaxCommentLength) {
		return Comment{}, ErrInvalidComment
	}
	var out Comment
	var target Approval
	err := m.d.Tx.WithinTx(ctx, func(ctx context.Context) error {
		var err error
		if target, err = m.load(ctx, id); err != nil {
			return err
		}
		if out, err = m.d.Store.AddComment(ctx, id, auth.PrincipalFrom(ctx).UserID, body); err != nil {
			return err
		}
		team := target.TeamID
		return m.d.Trail.Record(ctx, audit.Change{Action: audit.ActionApprovalCommented, Object: audit.ObjectDeployment,
			ObjectID: id.String(), TeamID: &team, After: map[string]any{"commentId": out.ID, "length": utf8.RuneCountInString(body)}})
	})
	if err != nil {
		return Comment{}, err
	}
	m.notify(ctx, EventCommented, target, body)
	return out, nil
}

// List returns one page of approvals with their destinations.
func (m *DefaultManager) List(ctx context.Context, f ListFilter, page Page) ([]Approval, *store.Cursor, error) {
	f.Caller = auth.PrincipalFrom(ctx).UserID
	items, next, err := m.d.Store.List(ctx, f, page)
	if err != nil {
		return nil, nil, err
	}
	if err := m.d.Destinations.Fill(ctx, items); err != nil {
		return nil, nil, err
	}
	out := make([]Approval, len(items))
	for i, a := range items {
		out[i] = a.Approval
	}
	return out, next, nil
}

// Get returns the review page: request, requested and active configurations, comments and caller abilities.
func (m *DefaultManager) Get(ctx context.Context, id uuid.UUID) (ApprovalDetail, error) {
	a, err := m.stored(ctx, id)
	if err != nil {
		return ApprovalDetail{}, err
	}
	active, err := m.d.Store.ActiveVersion(ctx, a.RouteID)
	if err != nil {
		return ApprovalDetail{}, err
	}
	comments, err := m.d.Store.Comments(ctx, id)
	if err != nil {
		return ApprovalDetail{}, err
	}
	eligible, err := m.d.Store.Approvers(ctx, a.TeamID, m.requesterID(a.Approval))
	if err != nil {
		return ApprovalDetail{}, err
	}
	return ApprovalDetail{Approval: a.Approval, Requested: VersionConfig{No: a.VersionNo, Config: a.Config}, Active: active,
		Comments: comments, CanDecide: m.canDecide(ctx, a.Approval), CanWithdraw: a.IsPending() && m.mayWithdraw(ctx, a.Approval) == nil,
		EligibleCount: len(eligible)}, nil
}

// decidable locks a pending request, checks If-Match, the approve permission and (for approvals) self-approval.
func (m *DefaultManager) decidable(ctx context.Context, id uuid.UUID, ifMatch store.RowVersion, approver uuid.UUID) (Approval, error) {
	a, err := m.pending(ctx, id)
	if err != nil {
		return Approval{}, err
	}
	if a.RowVersion != ifMatch {
		return Approval{}, &store.PreconditionFailedError{Current: a.RowVersion}
	}
	if err := m.d.Access.Authorize(ctx, rbac.PermDeploymentApprove, ResourceOf{}.Deployment(a.Approval)); err != nil {
		return Approval{}, err
	}
	if approver != uuid.Nil && a.RequestedByUser(approver) {
		return Approval{}, ErrSelfApproval
	}
	return a.Approval, nil
}

// pending locks the request and fails with 409 already_decided unless it is pending.
func (m *DefaultManager) pending(ctx context.Context, id uuid.UUID) (StoredApproval, error) {
	a, err := m.d.Store.Lock(ctx, id)
	if err != nil {
		return StoredApproval{}, err
	}
	if !a.IsPending() {
		return StoredApproval{}, Decided(a.Approval)
	}
	return a, nil
}

func (m *DefaultManager) mayWithdraw(ctx context.Context, a Approval) error {
	caller := auth.PrincipalFrom(ctx).UserID
	if a.RequestedByUser(caller) {
		return nil
	}
	access, err := m.d.Access.Effective(ctx, caller)
	if err != nil || !access.WorkspaceAdmin {
		return ErrNotRequester
	}
	return nil
}

func (m *DefaultManager) canDecide(ctx context.Context, a Approval) bool {
	if !a.IsPending() || a.RequestedByUser(auth.PrincipalFrom(ctx).UserID) {
		return false
	}
	return m.d.Access.Decide(ctx, rbac.PermDeploymentApprove, ResourceOf{}.Deployment(a)).Allowed
}

func (m *DefaultManager) requesterID(a Approval) uuid.UUID {
	if a.RequestedBy == nil {
		return uuid.Nil
	}
	return a.RequestedBy.ID
}

// stored reads one approval with its destinations.
func (m *DefaultManager) stored(ctx context.Context, id uuid.UUID) (StoredApproval, error) {
	a, err := m.d.Store.Get(ctx, id)
	if err != nil {
		return StoredApproval{}, err
	}
	items := []StoredApproval{a}
	if err := m.d.Destinations.Fill(ctx, items); err != nil {
		return StoredApproval{}, err
	}
	return items[0], nil
}

func (m *DefaultManager) load(ctx context.Context, id uuid.UUID) (Approval, error) {
	a, err := m.stored(ctx, id)
	return a.Approval, err
}

func (m *DefaultManager) record(ctx context.Context, action audit.Action, a Approval, id uuid.UUID, reason CancelReason) error {
	team := a.TeamID
	return m.d.Trail.Record(ctx, audit.Change{Action: action, Object: audit.ObjectDeployment, ObjectID: id.String(), TeamID: &team,
		After: ApprovalAudit{}.Of(a, reason)})
}

// notify publishes after commit; delivery failures are logged, never returned (the decision stands).
func (m *DefaultManager) notify(ctx context.Context, kind EventKind, a Approval, note string) {
	recipients, err := Recipients{Store: m.d.Store}.For(ctx, kind, a, auth.PrincipalFrom(ctx).UserID)
	if err == nil {
		err = m.d.Notifier.Publish(ctx, ApprovalEvent{Kind: kind, ApprovalID: a.ID, RouteID: a.RouteID, RouteName: a.RouteName,
			TeamID: a.TeamID, VersionNo: a.VersionNo, Status: a.Status, Actor: auth.PrincipalFrom(ctx).UserID, Note: note,
			Recipients: recipients, At: m.d.Now()})
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		m.d.Log.Warn(string(LogNotifyFailed), string(logging.AttrError), err)
	}
}

// ManagerLog names manager log messages.
type ManagerLog string

const LogNotifyFailed ManagerLog = "approval notification failed"
