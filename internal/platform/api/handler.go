// Package api holds the conventions shared by every HTTP handler: class-based handlers with a
// RouteSpec, the middleware chain, RFC 7807 errors, JSON decoding, cursor pages and If-Match.
package api

import (
	"net/http"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
)

// BodyLimit is the maximum accepted request body size in bytes.
type BodyLimit int64

const (
	BodyLimitDefault BodyLimit = 1 << 20
	BodyLimitLarge   BodyLimit = 10 << 20
)

// AuthPolicy tells the authenticate stage what to do when no credential matches.
type AuthPolicy string

const (
	AuthRequired AuthPolicy = "required"
	AuthOptional AuthPolicy = "optional"
	AuthNone     AuthPolicy = "none"
)

// RouteSpec declares how one handler is mounted and guarded.
type RouteSpec struct {
	Method      string
	Pattern     string
	Auth        AuthPolicy
	Permission  auth.Permission
	Object      auth.ObjectResolver
	Mutating    bool
	AuditAction audit.Action
	Versioned   bool
	BodyLimit   BodyLimit
}

// limit returns the configured body limit or the default.
func (s RouteSpec) limit() BodyLimit {
	if s.BodyLimit <= 0 {
		return BodyLimitDefault
	}
	return s.BodyLimit
}

// policy returns the auth policy, defaulting to required.
func (s RouteSpec) policy() AuthPolicy {
	if s.Auth == "" {
		return AuthRequired
	}
	return s.Auth
}

// Handler is the base of every class-based HTTP handler; returned errors become problems.
type Handler interface {
	Route() RouteSpec
	Serve(w http.ResponseWriter, r *http.Request) error
}

// Module groups one feature's handlers.
type Module interface {
	Handlers() []Handler
}

// Modules is a static Module built from a handler list.
type Modules []Handler

// Handlers implements Module.
func (m Modules) Handlers() []Handler {
	return m
}
