package approvals

import (
	"context"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// Activator is implemented by routes: Revalidate checks the requested version against today's connections,
// grants and test cases (read-only, before the decision transaction); Activate makes it live in the caller's tx.
type Activator interface {
	Revalidate(ctx context.Context, deploymentID uuid.UUID) error
	Activate(ctx context.Context, deploymentID, approver uuid.UUID) error
}

// Store persists approvals (deployments rows + comments).
type Store interface {
	List(ctx context.Context, f ListFilter, page Page) ([]StoredApproval, *store.Cursor, error)
	Get(ctx context.Context, id uuid.UUID) (StoredApproval, error)
	Lock(ctx context.Context, id uuid.UUID) (StoredApproval, error)
	ActiveVersion(ctx context.Context, route uuid.UUID) (*VersionConfig, error)
	SetDecisionNote(ctx context.Context, id uuid.UUID, note string) error
	Reject(ctx context.Context, id, by uuid.UUID, note string) error
	Cancel(ctx context.Context, id uuid.UUID, reason CancelReason) error
	CancelOtherPending(ctx context.Context, route, except uuid.UUID) ([]uuid.UUID, error)
	Comments(ctx context.Context, id uuid.UUID) ([]Comment, error)
	AddComment(ctx context.Context, id, author uuid.UUID, body string) (Comment, error)
	Approvers(ctx context.Context, team rbac.TeamID, excluding uuid.UUID) ([]uuid.UUID, error)
	Commenters(ctx context.Context, id uuid.UUID) ([]uuid.UUID, error)
	PendingByTeam(ctx context.Context) (map[rbac.TeamID]int, error)
}

// StoredApproval is an approval with the raw requested configuration (destinations are derived from it).
type StoredApproval struct {
	Approval
	Config []byte
}
