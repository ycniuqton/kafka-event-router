// Package identity holds the minimal users/teams/role_bindings repositories auth needs
// (provisioning, principal loading, first-admin bootstrap). access-control extends them.
package identity

import (
	"github.com/eventrouter/router/internal/store"
)

// LockID names transaction-scoped advisory locks used by identity writes.
type LockID int64

const BootstrapAdminLock LockID = 7002

// Repositories bundles the identity repositories over one DB.
type Repositories struct {
	Users        *UserRepository
	Teams        *TeamRepository
	RoleBindings *RoleBindingRepository
}

// New builds all identity repositories.
func New(db *store.DB) *Repositories {
	teams := &TeamRepository{db: db}
	bindings := &RoleBindingRepository{db: db}
	return &Repositories{
		Users:        &UserRepository{db: db, teams: teams, bindings: bindings},
		Teams:        teams,
		RoleBindings: bindings,
	}
}
