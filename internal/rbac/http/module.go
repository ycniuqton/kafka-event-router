// Package http serves the access-control administration API: users, invitations, role bindings, teams, grants and
// workspace settings.
// Handlers are class-based api.Handler values; permissions are declared on each RouteSpec.
package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/platform/problem"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// Path names the access-control routes.
type Path string

const (
	PathUsers        Path = "/api/v1/users"
	PathUser         Path = "/api/v1/users/{id}"
	PathRoleBindings Path = "/api/v1/role-bindings"
	PathRoleBinding  Path = "/api/v1/role-bindings/{userId}/{teamId}"
	PathInvitations  Path = "/api/v1/users/invitations"
	PathTeams        Path = "/api/v1/teams"
	PathTeam         Path = "/api/v1/teams/{id}"
	PathTeamGrants   Path = "/api/v1/teams/{id}/grants"
	PathTeamGrant    Path = "/api/v1/teams/{id}/grants/{connectionId}"
	PathSettings     Path = "/api/v1/workspace-settings"
)

// Param names path and query parameters.
type Param string

const (
	ParamID     Param = "id"
	ParamUserID Param = "userId"
	ParamTeamID Param = "teamId"
	ParamQuery  Param = "q"
	ParamTeam   Param = "team"
	ParamConnID Param = "connectionId"
)

// Deps groups what the handlers need; Passwords is set only in password mode (user creation, password reset).
type Deps struct {
	Users     *rbac.UserService
	Bindings  *rbac.BindingService
	Teams     *rbac.TeamService
	Grants    *rbac.GrantService
	Inviter   *rbac.Inviter
	Settings  *rbac.SettingsService
	Responder *api.Responder
	Pages     *api.PageParser
	Passwords *rbac.PasswordUserService
}

// NewModule returns the users and role-bindings handlers (plus password administration in password mode).
func NewModule(d Deps) api.Module {
	modules := baseHandlers(d)
	if d.Passwords != nil {
		modules = append(modules, &CreateUserHandler{d: d}, &ResetPasswordHandler{d: d})
	}
	return modules
}

func baseHandlers(d Deps) api.Modules {
	return api.Modules{
		&ListUsersHandler{d: d},
		&GetUserHandler{d: d},
		&PatchUserHandler{d: d},
		&ListBindingsHandler{d: d},
		&PutBindingHandler{d: d},
		&DeleteBindingHandler{d: d},
		&InviteUserHandler{d: d},
		&ListTeamsHandler{d: d},
		&GetTeamHandler{d: d},
		&CreateTeamHandler{d: d},
		&PatchTeamHandler{d: d},
		&DeleteTeamHandler{d: d},
		&ListGrantsHandler{d: d},
		&PutGrantHandler{d: d},
		&DeleteGrantHandler{d: d},
		&GetSettingsHandler{d: d},
		&PatchSettingsHandler{d: d},
	}
}

// UUIDReader parses ids from path and query parameters into 404 / 422 problems.
type UUIDReader struct{}

// ErrUnknownID is returned for malformed path ids (the object cannot exist).
var ErrUnknownID = problem.NewError(problem.KindNotFound, "The requested resource does not exist. It may have been deleted.")

// Path parses a path id.
func (UUIDReader) Path(raw string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, ErrUnknownID
	}
	return id, nil
}

// Query parses an optional query id; malformed values are 422 at /<name>.
func (UUIDReader) Query(r *http.Request, name Param) (*uuid.UUID, error) {
	raw := r.URL.Query().Get(string(name))
	if raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, api.NewValidationError().Add("/"+string(name), "uuid", "Use the id of an existing object.")
	}
	return &id, nil
}

// OptionalIfMatch reads If-Match when present (creations need none, changes need one).
type OptionalIfMatch struct{}

// Read returns nil when the header is absent.
func (OptionalIfMatch) Read(r *http.Request) (*store.RowVersion, error) {
	if r.Header.Get(string(api.HeaderIfMatch)) == "" {
		return nil, nil
	}
	v, err := api.Precondition{}.Require(r)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
