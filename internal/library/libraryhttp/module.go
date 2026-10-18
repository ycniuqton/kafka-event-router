// Package libraryhttp serves the transform library: /api/v1/transforms and /api/v1/lookup-tables
// (list, create, read, metadata, archive, versions, publish/import, usages, rows). Reads are open to every
// team; writes need library.write on the owner team.
package libraryhttp

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/eventrouter/router/internal/audit"
	"github.com/eventrouter/router/internal/auth"
	"github.com/eventrouter/router/internal/library"
	"github.com/eventrouter/router/internal/platform/api"
	"github.com/eventrouter/router/internal/rbac"
	"github.com/eventrouter/router/internal/store"
)

// Path names the library routes.
type Path string

const (
	PathTransforms          Path = "/api/v1/transforms"
	PathTransform           Path = "/api/v1/transforms/{id}"
	PathTransformVersions   Path = "/api/v1/transforms/{id}/versions"
	PathTransformVersion    Path = "/api/v1/transforms/{id}/versions/{version}"
	PathTransformUsages     Path = "/api/v1/transforms/{id}/usages"
	PathLookupTables        Path = "/api/v1/lookup-tables"
	PathLookupTable         Path = "/api/v1/lookup-tables/{id}"
	PathLookupTableVersions Path = "/api/v1/lookup-tables/{id}/versions"
	PathLookupTableVersion  Path = "/api/v1/lookup-tables/{id}/versions/{version}"
	PathLookupTableUsages   Path = "/api/v1/lookup-tables/{id}/usages"
	PathLookupTableRows     Path = "/api/v1/lookup-tables/{id}/versions/{version}/rows"
	ParamID                 Path = "id"
	ParamVersion            Path = "version"
	QueryText               Path = "q"
	QueryNames              Path = "names"
	QueryArchived           Path = "archived"
	QueryChangeNote         Path = "changeNote"
)

// Audit actions of library writes.
const (
	ActionTransformCreated          audit.Action = "transform.created"
	ActionTransformVersionPublished audit.Action = "transform.version_published"
	ActionTransformUpdated          audit.Action = "transform.updated"
	ActionTransformArchived         audit.Action = "transform.archived"
	ActionLookupCreated             audit.Action = "lookup_table.created"
	ActionLookupVersionPublished    audit.Action = "lookup_table.version_published"
	ActionLookupUpdated             audit.Action = "lookup_table.updated"
	ActionLookupArchived            audit.Action = "lookup_table.archived"
)

// Permissions: the library is open read (transform.read), writes need library.write.
var (
	PermRead  = rbac.PermTransformRead.Auth()
	PermWrite = rbac.PermLibraryWrite.Auth()
)

// KindRoutes binds one object kind to its paths, content member, audit actions and import support.
type KindRoutes struct {
	Kind       library.ObjectKind
	Collection Path
	Item       Path
	Versions   Path
	Version    Path
	Usages     Path
	Content    ContentMember
	Created    audit.Action
	Published  audit.Action
	Updated    audit.Action
	Archived   audit.Action
	Imports    bool
}

// KindRouteTable lists the served kinds (add an entry to serve a new kind).
var KindRouteTable = []KindRoutes{
	{Kind: library.KindTransform, Collection: PathTransforms, Item: PathTransform, Versions: PathTransformVersions,
		Version: PathTransformVersion, Usages: PathTransformUsages, Content: MemberSteps, Created: ActionTransformCreated,
		Published: ActionTransformVersionPublished, Updated: ActionTransformUpdated, Archived: ActionTransformArchived},
	{Kind: library.KindLookupTable, Collection: PathLookupTables, Item: PathLookupTable, Versions: PathLookupTableVersions,
		Version: PathLookupTableVersion, Usages: PathLookupTableUsages, Content: MemberRows, Created: ActionLookupCreated,
		Published: ActionLookupVersionPublished, Updated: ActionLookupUpdated, Archived: ActionLookupArchived, Imports: true},
}

// AuditActions lists every action of the module (for the audit catalog).
func AuditActions() []audit.Action {
	out := []audit.Action{}
	for _, k := range KindRouteTable {
		out = append(out, k.Created, k.Published, k.Updated, k.Archived)
	}
	return out
}

// Deps are the collaborators of the module.
type Deps struct {
	Service   *library.Service
	Pages     *api.PageParser
	Responder *api.Responder
}

// NewModule returns every handler of every kind plus the lookup rows endpoint.
func NewModule(d Deps) api.Module {
	mods := api.Modules{&RowsHandler{d: d}}
	for _, k := range KindRouteTable {
		mods = append(mods, &ListHandler{d: d, k: k}, &CreateHandler{d: d, k: k}, &GetHandler{d: d, k: k},
			&PatchHandler{d: d, k: k}, &ArchiveHandler{d: d, k: k}, &VersionsHandler{d: d, k: k},
			&VersionHandler{d: d, k: k}, &PublishHandler{d: d, k: k}, &UsagesHandler{d: d, k: k})
	}
	return mods
}

// OwnedObject is the object resolver of item routes: library.write on the object's team.
func (d Deps) OwnedObject(k KindRoutes) auth.ObjectResolver {
	return rbac.ObjectResolvers{}.Owned(rbac.ResourceType(k.Kind), string(ParamID), d.Service.Owner(k.Kind))
}

// ObjectID reads the {id} path parameter (a malformed id is not found).
type ObjectID struct{}

// Of parses the id.
func (ObjectID) Of(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, string(ParamID)))
	if err != nil {
		return uuid.Nil, store.ErrNotFound
	}
	return id, nil
}

// VersionNo reads the {version} path parameter (a malformed number is not found).
type VersionNo struct{}

// Of parses the version number.
func (VersionNo) Of(r *http.Request) (int, error) {
	n, err := strconv.Atoi(chi.URLParam(r, string(ParamVersion)))
	if err != nil || n < 1 {
		return 0, store.ErrNotFound
	}
	return n, nil
}

// OwnerTeamPolicy picks the owner team of a new object: the requested team when the caller can write
// there, else the caller's first team where they are editor or admin.
type OwnerTeamPolicy struct{}

// writerRoles may write to a team's library.
var writerRoles = map[auth.Role]bool{auth.RoleEditor: true, auth.RoleApprover: true, auth.RoleAdmin: true}

// Resolve returns the owner team or a 422 on /teamId.
func (OwnerTeamPolicy) Resolve(p auth.Principal, requested uuid.UUID) (uuid.UUID, error) {
	for _, t := range p.Teams {
		if writerRoles[t.Role] && (requested == uuid.Nil || t.TeamID == requested) {
			return t.TeamID, nil
		}
	}
	msg := library.MsgTeamRequired
	if requested != uuid.Nil {
		msg = library.MsgTeamNotMember
	}
	return uuid.Nil, api.NewValidationError().Add(string(library.FieldOwner), string(library.CodeDenied), string(msg))
}
