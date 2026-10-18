package auth

import "context"

// Access is the coarse access level of a permission, used by the bootstrap authorizer.
type Access string

const (
	AccessRead  Access = "read"
	AccessWrite Access = "write"
	AccessAdmin Access = "admin"
)

// Permission is a named capability a route requires; access-control's RBAC decides on Key.
// The zero Permission means "any authenticated principal".
type Permission struct {
	Key    string
	Access Access
}

// IsZero reports whether no permission is required beyond authentication.
func (p Permission) IsZero() bool {
	return p.Key == ""
}

// ObjectRef identifies the object a permission is checked against (empty for collection routes).
type ObjectRef struct {
	Type   string
	ID     string
	TeamID string
}

// Authorizer is the PORT implemented by access-control (internal/rbac); platform-core only enforces it.
type Authorizer interface {
	Authorize(ctx context.Context, p Principal, perm Permission, obj ObjectRef) error
}
