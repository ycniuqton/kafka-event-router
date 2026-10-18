package rbac

// DenyReason tells why a decision denied access (also the 403 problem code).
type DenyReason string

const (
	ReasonNone              DenyReason = ""
	ReasonNoBinding         DenyReason = "no_binding"
	ReasonMissingPermission DenyReason = "missing_permission"
	ReasonDisabledUser      DenyReason = "disabled_user"
	ReasonEngineError       DenyReason = "engine_error"
	ReasonUnknownPermission DenyReason = "unknown_permission"
)

// Decision is the outcome of one authorization check.
type Decision struct {
	Allowed bool
	Reason  DenyReason
	Missing Permission
}

// Allow is the allowing decision.
func Allow() Decision {
	return Decision{Allowed: true}
}

// Deny builds a denying decision for permission p.
func Deny(reason DenyReason, p Permission) Decision {
	return Decision{Reason: reason, Missing: p}
}

// Visibility tells whether sample payloads are shown raw or masked.
type Visibility string

const (
	VisibilityRaw    Visibility = "raw"
	VisibilityMasked Visibility = "masked"
)

// TeamAccess is the caller's role and permissions in one team.
type TeamAccess struct {
	Role        Role         `json:"role"`
	Permissions []Permission `json:"permissions"`
}

// Access is the effective permission summary embedded in GET /auth/me (UI gating; the server stays the authority).
type Access struct {
	WorkspaceAdmin bool                  `json:"workspaceAdmin"`
	Workspace      []Permission          `json:"workspace"`
	Teams          map[string]TeamAccess `json:"teams"`
}
