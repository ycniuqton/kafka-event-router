# access-control — Design (00000-init)

Read with `01-requirements.md`. Names reuse the existing skeleton (`internal/store`, `internal/platform/problem`,
migration `0001_init`) and platform-core's `auth.Principal` / `auth.Authorizer` / audit recorder.

## 1. Overview

```
            ┌──────────── cmd/api (chi) ─────────────────────────────────────────────┐
 request ─▶ │ platform-core: authn ─▶ AuditContext ─▶ Authorizer port ─┐             │
            │                                                         ▼             │
            │   internal/rbac  Manager ─▶ PolicyEngine (rolebinding | opa[P3])        │
            │        │  SubjectLoader ─▶ SubjectCache ◀── NOTIFY rbac_changed         │
            │        ▼                                                                │
            │   feature handlers (routes, connections, preview proxy, dlq, ...)       │
            │        │ Gate()                       │ RecordTx()  (platform recorder) │
            │        ▼                              ▼                                 │
            │   internal/approvals Manager      audit_log (append-only)               │
            │        │ Activator port ─▶ routes   ▲  internal/audit Manager (read,    │
            │        │ Notifier port  ─▶ monitoring  │  catalog, export sinks)        │
            └────────┴────────────────────────────────┴────────────────────────────────┘
```

Three Go packages, each with one Manager as the only entry point for business code:

| Package | Manager | Extensible domain (base interface → implementations, config-selected) |
| --- | --- | --- |
| `internal/rbac` | `rbac.Manager` | `PolicyEngine` → `RoleBindingEngine` (default), `opa.Engine` [P3, `internal/rbac/opa`] via `ROUTER_RBAC_ENGINE` |
| `internal/approvals` | `approvals.Manager` | `ApprovalPolicy` → `TeamProductionPolicy` (default) via `ROUTER_APPROVAL_POLICY` |
| `internal/audit` | `audit.Manager` | `ExportSink` → `NoopExportSink`, `WebhookExportSink`, `SyslogExportSink` [P3] via `ROUTER_AUDIT_EXPORT_SINK`; `Exporter` → `JSONLExporter`, `CSVExporter` by `?format=` |

Selection follows the `secrets.Registry` pattern already in the repo: `Registry.Register(kind, factory)` +
`Registry.Build(kind)`, never `switch` on concrete types.

## 2. Domain model

### 2.1 Roles, scopes, permissions (`internal/rbac/role.go`, `permission.go`)

```go
type Role string            // matches Postgres enum team_role
const (RoleViewer Role = "viewer"; RoleEditor Role = "editor"; RoleApprover Role = "approver"; RoleAdmin Role = "admin")

type Scope string
const (ScopeWorkspace Scope = "workspace"; ScopeTeam Scope = "team")

type Permission string      // "<object>.<verb>"; full catalog below, one constant each
```

Role → permission matrix (`RoleMatrix`, a `map[Role]PermissionSet` declared once; Approver = Editor ∪ approve):

| Permission (team scope) | Viewer | Editor | Approver | Admin |
| --- | --- | --- | --- | --- |
| `route.read`, `connection.read`, `transform.read`, `metrics.read`, `version.read` | ✓ | ✓ | ✓ | ✓ |
| `route.write` (create, draft, pause/resume, archive, import, rollback request) | | ✓ | ✓ | ✓ |
| `route.deploy_nonprod` | | ✓ | ✓ | ✓ |
| `route.deploy_prod` (direct, still subject to team approval policy) | | | ✓ | ✓ |
| `deployment.approve` | | | ✓ | ✓ |
| `transform.write`, `library.write`, `testcase.write` | | ✓ | ✓ | ✓ |
| `sample.read` (masked for Viewer) | ✓ | ✓ | ✓ | ✓ |
| `sample.read_raw` | | ✓ | ✓ | ✓ |
| `dlq.read` / `dlq.act` (retry, discard, export) | ✓ / | ✓ / ✓ | ✓ / ✓ | ✓ / ✓ |
| `livetail.read`, `alert.read` / `alert.write` | ✓ / | ✓ / ✓ | ✓ / ✓ | ✓ / ✓ |
| `connection.write`, `connection.secret_write`, `topic.create` | | | | ✓ |
| `team.members_manage` | | | | ✓ |

| Permission (workspace scope, workspace admin only) |
| --- |
| `user.manage`, `team.manage`, `grant.manage`, `settings.manage`, `audit.read`, `audit.export` |

Workspace admin implies `RoleAdmin` on every team. Read permissions are granted workspace-wide to any user
with at least one binding (tech lead "see what flows where"); writes are team-scoped (decision: users can read other teams' routes/connections).

### 2.2 Core types

```go
// internal/rbac/subject.go
type SubjectKind string        // user | token | service (mirrors platform-core auth.Principal.Kind)
type Subject struct {
    UserID         uuid.UUID
    Kind           SubjectKind
    WorkspaceAdmin bool
    Bindings       map[TeamID]Role
    Disabled       bool
}

// internal/rbac/resource.go
type ResourceType string       // route | connection | transform | lookup_table | deployment | team | user | audit | settings
type Resource struct {
    Type      ResourceType
    ID        string
    TeamID    TeamID           // zero for workspace-scoped resources
    EnvTag    EnvTag           // prod | staging | dev (connections, deploy targets)
    Sensitive bool             // connections.sensitive
}

// internal/rbac/decision.go
type DenyReason string         // no_binding | missing_permission | disabled_user | engine_error | unknown_permission
type Decision struct { Allowed bool; Reason DenyReason; Missing Permission }

type Visibility string         // raw | masked
type Access struct {           // embedded in GET /auth/me
    WorkspaceAdmin bool
    Workspace      []Permission
    Teams          map[TeamID]TeamAccess   // {role, permissions}
}
```

### 2.3 Interfaces

```go
// internal/rbac/engine.go — base interface for the extensible policy domain
type PolicyEngine interface {
    Kind() EngineKind
    Evaluate(ctx context.Context, s Subject, p Permission, r Resource) Decision
    Permissions(s Subject, team TeamID) PermissionSet
}

// internal/rbac/manager.go — only entry point; implements platform-core auth.Authorizer
type Manager interface {
    Authorize(ctx context.Context, p Permission, r Resource) error          // ErrForbidden wraps Decision
    Decide(ctx context.Context, p Permission, r Resource) Decision
    Effective(ctx context.Context, userID uuid.UUID) (Access, error)
    SampleVisibility(ctx context.Context, conn Resource) (Visibility, error)
    CanUseConnection(ctx context.Context, team TeamID, connID uuid.UUID) (bool, error) // grants [P2]
}

type SubjectLoader interface { Load(ctx context.Context, userID uuid.UUID) (Subject, error) }
type SubjectCache  interface { Get(uuid.UUID) (Subject, bool); Put(Subject); Invalidate(uuid.UUID); InvalidateAll() }
```

```go
// internal/approvals
type DeployTarget struct {
    RouteID uuid.UUID; TeamID rbac.TeamID; VersionID uuid.UUID
    Destinations []rbac.Resource   // one per branch destination connection
}
type GateOutcome string            // deploy_now | require_approval | deny
type GateResult struct { Outcome GateOutcome; Reason GateReason; Approvers int }

type ApprovalPolicy interface {     // base interface, config-selected
    Kind() PolicyKind
    Evaluate(ctx context.Context, s rbac.Subject, t DeployTarget, team TeamPolicy) GateResult
}

type Activator interface {          // implemented by internal/routes (port, avoids import cycle)
    Activate(ctx context.Context, deploymentID uuid.UUID) error             // re-validates + switches active version, in caller's tx
}
type Notifier interface {           // implemented by monitoring's event publisher
    Publish(ctx context.Context, e ApprovalEvent) error
}

type Manager interface {
    Gate(ctx context.Context, t DeployTarget) (GateResult, error)
    Request(ctx context.Context, t DeployTarget, note string) (Approval, error)  // called by routes inside its deploy tx
    Approve(ctx context.Context, id uuid.UUID, note string, ifMatch RowVersion) (Approval, error)
    Reject(ctx context.Context, id uuid.UUID, note string, ifMatch RowVersion) (Approval, error)
    Withdraw(ctx context.Context, id uuid.UUID) (Approval, error)
    Comment(ctx context.Context, id uuid.UUID, body string) (Comment, error)
    List(ctx context.Context, f ListFilter, page store.Page) ([]Approval, string, error)
    Get(ctx context.Context, id uuid.UUID) (ApprovalDetail, error)
}
```

```go
// internal/audit
type Action string        // catalog: route.created, route.draft_saved, route.deployed, route.deploy_requested,
                          // route.paused, route.resumed, route.rolled_back, route.archived, route.imported,
                          // connection.created, connection.updated, connection.deleted, connection.secret_replaced,
                          // connection.tested, topic.created, transform.saved, library.published,
                          // deployment.approved, deployment.rejected, deployment.withdrawn, approval.commented,
                          // role_binding.set, role_binding.removed, user.updated, team.created, team.updated,
                          // team.deleted, grant.added, grant.removed, settings.updated,
                          // sample.viewed, sample.viewed_raw, livetail.opened, dlq.viewed, dlq.retried,
                          // dlq.discarded, dlq.exported, secret.read_by_service, audit.exported
type ObjectType string    // route | connection | topic | transform | lookup_table | deployment | team | user | role_binding | settings | sample_session | dlq
type ActionMeta struct { Sensitive bool; Category Category }   // Category: change | access | admin
var Catalog map[Action]ActionMeta                               // single source; unknown action → recorder error

type Filter struct {
    Actor *uuid.UUID; Actions []Action; ObjectType *ObjectType; ObjectID *string
    TeamID *uuid.UUID; From, To *time.Time; SensitiveOnly bool; Text string
}
type ExportSink interface { Kind() SinkKind; Send(ctx context.Context, batch []Entry) error }   // base interface
type Exporter interface   { Kind() FileFormat; Write(w io.Writer, it EntryIterator) error }   // jsonl | csv

type Manager interface {
    Query(ctx context.Context, f Filter, page store.Page) ([]Entry, string, error)
    Export(ctx context.Context, f Filter, format FileFormat, w io.Writer) error
    Describe(a Action) (ActionMeta, bool)
}
```

Writes stay in platform-core's recorder (`RecordTx`), which validates `Action` against `audit.Catalog` and
stores `sensitive` from `ActionMeta`. access-control owns the catalog and the read side only.

### 2.4 HTTP handlers (class-based, `internal/<pkg>/http/`)

`rbac/http`: `UsersHandler`, `RoleBindingsHandler`, `TeamsHandler`, `GrantsHandler`, `SettingsHandler`.
`approvals/http`: `ApprovalsHandler`. `audit/http`: `AuditLogHandler`.
Each handler struct has `Mount(r chi.Router)` and one method per endpoint; permission requirements are declared
in a table `[]EndpointRule{Method, Pattern, Permission, ResourceResolver}` consumed by platform-core's Authorizer
middleware, so no handler hand-codes checks.

## 3. Data model touchpoints

Existing (`0001_init`): `teams`, `users`, `role_bindings(user_id, team_id, role)`, `deployments(status, requested_by,
approved_by, approved_at)`, `audit_log` (append-only trigger). platform-core adds `row_version` (0002) and
`audit_log.request_id, team_id, actor_kind` (0003).

New migrations owned here (MVP number fixed at 0005; P2/P3 numbers ≥ 0010, next free at implementation time):

| Migration | Phase | Changes |
| --- | --- | --- |
| `0005_access_control_core` | MVP | `users.is_workspace_admin bool NOT NULL DEFAULT false`, `users.disabled_at timestamptz`; `audit_log.sensitive bool NOT NULL DEFAULT false`, `audit_log.object_label text` (human label survives deletes); indexes `audit_log (sensitive, at DESC) WHERE sensitive`, `audit_log (action, at DESC)`; trigger `NOTIFY rbac_changed` on `role_bindings` and `users` (is_workspace_admin, disabled_at) changes |
| `00NN_access_control_teams` | P2 | `teams.require_prod_approval bool NOT NULL DEFAULT true`; `team_connection_grants(team_id, connection_id, granted_by, created_at, PK(team_id, connection_id))`; `users.invited_at`, `users.invited_by` (pending users: `sso_subject IS NULL`) |
| `00NN_access_control_approvals` | P2 | `ALTER TYPE deployment_status ADD VALUE 'rejected'`, `ADD VALUE 'cancelled'` (own migration, no tx); `deployments.decision_note text`, `rejected_by uuid`, `rejected_at timestamptz`, `cancel_reason text`, `row_version bigint`; partial unique index `deployments (route_id) WHERE status = 'pending_approval'`; `approval_comments(id, deployment_id, author_id, body, created_at)` |
| `2010_access_control_workspace` | P3 | `workspace_settings(key text PK, value jsonb, updated_by, created_at, updated_at, row_version)` + bump trigger; `audit_export_checkpoints(sink text PK, last_id, exported_total, last_error, last_attempt_at, updated_at)`; `pg_trgm` (schema `public`) + GIN trigram index on `audit_log.object_label` |
| `2011_access_control_audit_search` | P3 | GIN trigram on `audit_log.object_id` and `action`; btree `audit_log (object_id)` (text search, see 5.8) |

`connections.sensitive` is not migrated here (owned by connections `0004`).
`approvals` has no table of its own: an approval **is** a `deployments` row in `pending_approval` plus comments.

`WorkspaceSettingKey` enum: `ai_assist_enabled` (P3, default false), `default_require_prod_approval` (P2).

## 4. API (all under `/api/v1`, RFC 7807, cursor pages `{items, next_cursor}`, `If-Match` on mutations)

| Method & path | Perm | Phase | Request → Response |
| --- | --- | --- | --- |
| `GET /users?q=&team=&cursor=` | `user.manage` or any member (read) | MVP | → `{items:[User], next_cursor}` |
| `GET /users/{id}` | member | MVP | → `User` + ETag |
| `PATCH /users/{id}` | `user.manage` | MVP | `{displayName?, workspaceAdmin?, disabled?}` → `User`; 409 `last_admin` |
| `POST /users/invitations` | `user.manage` | P2 | `{email, displayName, teamId, role}` → `201 User{status:"invited"}` |
| `GET /role-bindings?userId=&teamId=` | member | MVP | → `{items:[RoleBinding]}` |
| `PUT /role-bindings/{userId}/{teamId}` | `team.members_manage` on team | MVP | `{role}` → `RoleBinding`; 422 role not assignable in phase |
| `DELETE /role-bindings/{userId}/{teamId}` | `team.members_manage` | MVP | → 204; 409 `last_admin` |
| `GET /teams`, `GET /teams/{id}` | member | P2 | → `Team{id,name,requireProdApproval,memberCount,rowVersion}` |
| `POST /teams`, `PATCH /teams/{id}`, `DELETE /teams/{id}` | `team.manage` | P2 | `{name, requireProdApproval}`; delete 409 `team_not_empty` |
| `GET /teams/{id}/grants`, `PUT/DELETE /teams/{id}/grants/{connectionId}` | `grant.manage` | P2 | → `{items:[{connectionId, grantedBy, createdAt}]}` |
| `GET /approvals?status=&teamId=&routeId=&mine=` | `route.read` | P2 | → `{items:[Approval]}` |
| `GET /approvals/{id}` | `route.read` | P2 | → `ApprovalDetail{approval, route, version, activeVersionNo, comments[], canDecide}` |
| `POST /approvals/{id}/approve` | `deployment.approve` | P2 | `{note?}` + If-Match → `Approval{status:"rolling_out"}` |
| `POST /approvals/{id}/reject` | `deployment.approve` | P2 | `{note}` (required) → `Approval{status:"rejected"}` |
| `POST /approvals/{id}/withdraw` | requester | P2 | → `Approval{status:"cancelled", cancelReason:"withdrawn"}` |
| `POST /approvals/{id}/comments` | `route.read` | P2 | `{body}` → `201 Comment` |
| `GET /workspace-settings`, `PATCH /workspace-settings` | `team.read` / `settings.manage` | P3 | `{aiAssistEnabled, defaultRequireProdApproval, rowVersion}` + ETag; PATCH If-Match (aggregate version) |
| `GET /audit-log?actor=&action=&objectType=&objectId=&teamId=&from=&to=&sensitive=&q=&cursor=&limit=` | `audit.read` | MVP (pulled from P3 for the audit screen) | → `{items:[AuditEntry], next_cursor}` |
| `GET /audit-log/actions` | `audit.read` | MVP | → `{items:[{action, category, sensitive}]}` (filter bar) |
| `GET /audit-log/export?format=jsonl\|csv&...filters` | `audit.export` | P3 | streamed file (newest first, ≤ 1 M rows); itself audited `audit.exported` |
| `GET /audit-log/export/status` | `audit.read` | P3 | → `{enabled, sink, target (no credentials), checkpoint, lagRows}` (Settings → Workspace) |

Contributed to other endpoints: `GET /auth/me` (platform-core) gains `access: Access`; `POST /routes/{id}/deploy`
(routes) returns `202 {deployment, approval}` when gated.

Shapes:

```jsonc
User         { "id","email","displayName","workspaceAdmin","status":"active|invited|disabled","teams":[{"teamId","teamName","role"}],"lastLoginAt","rowVersion" }
RoleBinding  { "userId","teamId","role":"viewer|editor|approver|admin","createdAt" }
Approval     { "id","routeId","routeName","teamId","versionNo","status":"pending_approval|rolling_out|active|failed|rejected|cancelled",
               "requestedBy":{"id","name"},"requestedAt","changeNote","decidedBy","decidedAt","decisionNote","destinations":[{"connectionId","name","envTag"}],"rowVersion" }
AuditEntry   { "id","at","actor":{"id","name","kind"},"action","category","objectType","objectId","objectLabel","teamId","ip","requestId","sensitive","diff" }
Access       { "workspaceAdmin":true,"workspace":["user.manage",...],"teams":{"<teamId>":{"role":"editor","permissions":["route.write",...]}} }
Problem 403  { "type":".../problems/forbidden","status":403,"detail":"You need the Approver role in team Payments to approve deploys. Ask an admin.","missingPermission":"deployment.approve" }
```

## 5. Flows

### 5.1 Authorize an API call

```
HTTP request /api/v1/... (any)
└─ platform-core authn middleware → auth.Principal in ctx (401 if none)
   └─ Authorizer middleware looks up EndpointRule for (method, pattern)
      ├─ no rule registered → deny 403 unknown_permission (fail closed; caught by test)
      └─ ResourceResolver loads Resource{type, id, teamId, envTag, sensitive}
         ├─ object not found → 404 problem
         └─ rbac.Manager.Authorize(perm, resource)
            └─ SubjectCache.Get(userID)
               ├─ hit → use cached Subject
               └─ miss → SubjectLoader reads users + role_bindings → cache.Put (TTL 30 s)
                  └─ PolicyEngine.Evaluate(subject, perm, resource)
                     ├─ subject disabled → deny disabled_user → 403
                     ├─ workspace admin → allow
                     ├─ perm is workspace-scoped and not admin → deny missing_permission → 403
                     ├─ binding on resource.teamId has perm → allow → handler runs
                     ├─ read perm and any binding exists → allow (workspace-wide read)
                     └─ otherwise → deny missing_permission → 403 problem {missingPermission}
```

---

### 5.2 Role binding change (Settings → Members & roles)

```
Admin changes role in Settings (PUT /role-bindings/{userId}/{teamId}, If-Match)
└─ Authorizer: team.members_manage on team
   └─ RoleBindingsHandler.Put → rbac.BindingService.Set (in tx)
      ├─ role not assignable in current phase (MVP: admin|editor) → 422 path=/role
      ├─ stale If-Match → 412
      ├─ demotes the last admin of the workspace → 409 last_admin
      └─ upsert role_bindings → audit role_binding.set {before, after} (same tx)
         └─ DB trigger → NOTIFY rbac_changed {userId}
            └─ [event] every API instance's InvalidationListener → SubjectCache.Invalidate(userId)
               └─ [event] monitoring WS /events → user's open tabs refetch /auth/me → UI re-gates
```

---

### 5.3 First login / bootstrap admin

```
OIDC callback or dev login (platform-core) upserts users row
└─ calls rbac.Bootstrapper.OnLogin(user)
   ├─ first user ever, email in ROUTER_BOOTSTRAP_ADMIN_EMAILS, or dev-login seed → is_workspace_admin = true
   │  + admin on default team → audit user.updated (actor=system)
   ├─ pending invited user matched by email (P2) → attach sso_subject, keep invited bindings → status active
   └─ no binding → no access except /auth/me → UI shows "Ask an admin to add you to a team"
      (roles are assigned manually by an admin; OIDC group mapping later via platform-core PC-79)
```

---

### 5.4 Deploy gate (routes calls approvals)

```
User clicks Deploy / Request approval (POST /routes/{id}/deploy, routes feature)
└─ routes validates draft (routes FR-31)
   └─ approvals.Manager.Gate(DeployTarget{team, destinations})
      └─ ApprovalPolicy(TeamProductionPolicy).Evaluate
         ├─ no prod destination
         │  ├─ subject has route.deploy_nonprod → deploy_now
         │  └─ otherwise → deny → 403
         └─ some destination env_tag = prod
            ├─ team.require_prod_approval = false and subject has route.deploy_prod → deploy_now
            ├─ subject has route.write → require_approval
            └─ otherwise → deny → 403
   ├─ deploy_now → routes creates version + deployment rolling_out (routes FR-32) → audit route.deployed
   └─ require_approval → routes creates immutable version, then approvals.Manager.Request (same tx)
      ├─ older pending request for route → status cancelled, cancel_reason superseded → audit deployment.withdrawn
      ├─ insert deployments status pending_approval, requested_by → audit route.deploy_requested
      ├─ no eligible approver in team (excluding requester) → still pending, GateReason no_approver → UI warning
      └─ [async] after commit → Notifier.Publish(approval.requested) → monitoring bell for team approvers + workspace admins
         └─ 202 {deployment, approval} → builder shows "Waiting for approval" banner
```

---

### 5.5 Approve

```
Approver clicks Approve on review page (POST /approvals/{id}/approve, If-Match)
└─ Authorizer: deployment.approve on deployment's team
   └─ approvals.Manager.Approve (tx, SELECT ... FOR UPDATE on deployment)
      ├─ status ≠ pending_approval → 409 already_decided {status, decidedBy}
      ├─ row_version ≠ If-Match → 412
      ├─ approver = requester → 403 self_approval
      └─ set approved_by, approved_at, decision_note
         └─ Activator.Activate(deploymentId) (routes, same tx)
            ├─ re-validation fails (connection deleted, grant revoked, test case fails) → tx rollback → 422 {errors[] with path}
            └─ active_version_id switched, status rolling_out, NOTIFY route change → audit deployment.approved
               └─ [async] after commit → Notifier.Publish(approval.decided) → requester bell "Approved by Minh N."
                  └─ [event] route-runtime rollout → deployment active | failed (routes FR-35)
```

---

### 5.6 Reject / withdraw / comment

```
Approver clicks Reject (POST /approvals/{id}/reject {note})
└─ Authorizer: deployment.approve
   └─ approvals.Manager.Reject (tx, FOR UPDATE)
      ├─ note empty → 422 path=/note
      ├─ status ≠ pending_approval → 409 already_decided
      └─ status rejected, rejected_by/at, decision_note → audit deployment.rejected
         └─ [async] Notifier.Publish(approval.decided) → requester bell with note; draft stays editable
---
Requester clicks Withdraw (POST /approvals/{id}/withdraw)
└─ approvals.Manager.Withdraw
   ├─ caller ≠ requester and not workspace admin → 403
   ├─ status ≠ pending_approval → 409
   └─ status cancelled, cancel_reason withdrawn → audit deployment.withdrawn → [async] notify approvers (clears bell item)
---
Anyone with route.read posts comment (POST /approvals/{id}/comments {body})
└─ approvals.Manager.Comment
   ├─ body empty or > 4000 chars → 422 path=/body
   └─ insert approval_comments → audit approval.commented → [async] notify requester + prior commenters
```

---

### 5.7 Sample visibility (preview, live tail, DLQ)

```
User loads samples (POST /preview/samples) / opens live tail / opens a DLQ entry
└─ owning handler resolves connection Resource{teamId, sensitive}
   └─ rbac.Manager.Authorize(sample.read | livetail.read | dlq.read)
      └─ rbac.Manager.SampleVisibility(connection)
         ├─ subject is Viewer on that team → masked (always)
         ├─ connection.sensitive (any other role) → masked
         └─ not sensitive and has sample.read_raw (Editor/Approver/Admin) → raw → audit sample.viewed
      └─ owner applies its type-preserving masker when masked (live-preview / monitoring / dead-letters) → audit sample.viewed {masked}
```

---

### 5.8 Audit log screen query

```
User opens Audit log / changes a filter (GET /audit-log?..., cursor)
└─ Authorizer: audit.read (workspace admin)
   └─ AuditLogHandler parses Filter
      ├─ unknown action / bad time range → 422 path=<param>
      └─ audit.Manager.Query → keyset on (at DESC, id DESC), limit ≤ 200
         └─ join users for actor name; label = stored object_label, else live name (ObjectLabeler)
            ├─ object still exists → route / connection / user / team / "route vN" name
            ├─ object deleted, no stored label → FE shows "<type> <id>"
            └─ {items, next_cursor} → table appends rows ("Load more"); sensitive rows amber
---
q=<text> filter
└─ WhereBuilder{Search: ObjectLabeler.Search()}
   ├─ stored object_label / object_id / action ILIKE → trigram indexes (2010, 2011)
   ├─ empty label and live object name matches → object_id = ANY(finder ids) (routes, connections, users, teams, versions, …)
   └─ empty label, type without finder (sample_session) → resolved label ILIKE, row by row (object_type index)
```

---

### 5.9 Audit export to SIEM

```
[cron] AuditExportJob every 10 s (API leader only, ROUTER_AUDIT_EXPORT_SINK ≠ none)
└─ read audit_export_checkpoints.last_id for sink
   └─ select audit_log where id > last_id order by id limit 500
      ├─ no rows → sleep
      └─ ExportSink.Send(batch)
         ├─ success → checkpoint last_id = max(id) → metric router_audit_exported_total
         └─ failure → keep checkpoint, backoff (≤ 5 min) → metric router_audit_export_errors_total → monitoring platform alert
---
User clicks Export on Audit log (GET /audit-log/export?format=csv&filters)
└─ Authorizer: audit.export
   ├─ unknown format / bad filter → 422 path=/format | /<param>
   └─ audit.exported {format, filter} recorded (sensitive)
      └─ audit.Manager.Export → Exporter(format) streams keyset pages newest first (≤ 1 M rows), CSV formula cells escaped
---
Settings → Workspace opens (GET /audit-log/export/status)
└─ ExportMonitor: sink none → "Not configured"
   └─ checkpoint + lag → healthy | catching up (lag > 0) | failing (last_error kept until the next delivered batch)
```

---

### 5.10 FE permission gating and deploy button

```
App start (FE)
└─ SessionBootstrap GET /auth/me (platform-core) → AccessStore.set(access)
   └─ useCan(permission, teamId) / <Can> components read AccessStore
      ├─ allowed → control enabled
      └─ denied → control hidden (nav) or disabled with tooltip "Requires Editor role"
---
Builder review step renders the deploy button (routes UI)
└─ DeployModeResolver(access, route.teamId, destination envTags, team.requireProdApproval)
   ├─ no prod destination and can route.deploy_nonprod → "Deploy"
   ├─ prod and (team does not require approval) and can route.deploy_prod → "Deploy"
   ├─ prod and can route.write → "Request approval"
   └─ else → disabled "You can't deploy this route"
   └─ server response is authoritative: 202 pending → banner; 403 → toast with problem detail
---
Any API call answers 403 (role changed since the UI gated it)
└─ HttpClient → ApiErrorBus.publish
   └─ ForbiddenToast (access/) → toast(problem.detail) → accessStore.refresh() → gated controls re-render
---
api start (cmd/api Mount)
└─ AccessWiring.VerifyCoverage(public modules)
   ├─ route permission not in PermissionCatalog → log error (engine denies it at request time: fail closed)
   ├─ route without permission and not in PublicEndpoints → log error
   └─ cmd/api/coverage_test.go runs the same check over every feature module in CI
```

## 6. Frontend

Location and conventions follow `web/ARCHITECTURE.md` (React 19 + TS, CSS Modules + tokens, services from `src/api` only).

| Path | Contents |
| --- | --- |
| `web/src/domain/member.ts` | extend: `Role` stays display enum; add `RoleKey` (`viewer`…) + mapping; `User`, `RoleBinding`, `UserStatus` |
| `web/src/domain/team.ts` | `Team`, `ConnectionGrant` |
| `web/src/domain/access.ts` | `Permission` const object (mirrors Go catalog), `Access`, `DeployMode` |
| `web/src/domain/approval.ts` | `ApprovalStatus`, `Approval`, `ApprovalDetail`, `ApprovalComment` |
| `web/src/domain/audit.ts` | extend `AuditEntry` (`actorId`, `objectType`, `objectId`, `category`, `requestId`); `AuditFilter`, `AuditAction` |
| `web/src/domain/settings.ts` | add `WorkspaceSettings` |
| `web/src/api/contracts.ts` | `AccessService` (me), `MembersService` (list, setRole, removeBinding, update, invite), `TeamsService`, `ApprovalsService`, `AuditService.query(filter, cursor)` + `exportUrl`, `WorkspaceSettingsService` |
| `web/src/api/mock/MockAccessServices.ts` | mock implementations over `src/mocks/{members,audit,approvals,teams}.ts`, enforcing the same role matrix |
| `web/src/access/` | `AccessStore`, `useCan`, `<Can>`, `DeployModeResolver`, `RoleBadge` |
| `web/src/pages/settings/` | `SettingsPage` → `AppearanceSection`, `MembersSection` (+ `RoleSelect`, `InviteDialog` P2), `TeamsSection` (P2: `TeamEditor`, `GrantsEditor`, approval toggle), `WorkspaceSection` (P3) |
| `web/src/pages/audit/` | `AuditLogPage`, `AuditFilterBar`, `AuditTable` (virtualised, cursor), `AuditRowDetail` (diff), `ExportButton` (P3) |
| `web/src/pages/approvals/` | `ApprovalReviewPage` at `/approvals/:id` (no nav entry; opened from bell and route History), `ApprovalList` drawer, `DecisionBar`, `CommentThread`; diff via routes' `VersionDiff` |

Nav: Audit log (P3) and Settings (MVP) exist already in the shell (7 entries). Audit log nav item is hidden
unless `audit.read`.

## 7. Error cases

| Case | Response |
| --- | --- |
| No session | 401 (platform-core) |
| Missing permission | 403 `forbidden` + `missingPermission`, message "what / why / what to do" |
| Disabled user with live session | 403 `disabled_user`; session revoked on next request |
| Demote/remove/disable last workspace admin | 409 `last_admin` |
| Role not assignable in current phase | 422 `path: /role` |
| Delete team that owns routes/connections | 409 `team_not_empty` with counts |
| Approve/reject already-decided request | 409 `already_decided` with current status |
| Self-approval | 403 `self_approval` |
| Re-validation fails on approve | 422 with `errors[]` paths; request stays pending |
| Stale `If-Match` | 412; missing → 428 |
| Route uses connection not granted to its team | validation error `path: /source/connectionId` (raised by routes via `CanUseConnection`) |
| Policy engine / subject load error | 403 `engine_error` (fail closed) + error log + metric `router_rbac_errors_total` |
| Unknown audit action on write | recorder error → request fails (tests catch missing catalog entries) |
| SIEM sink down | export lags, checkpoint retained, metric + log; API unaffected |

## 8. Metrics

`router_rbac_decisions_total{permission,outcome}`, `router_rbac_cache_hits_total`, `router_rbac_errors_total`,
`router_approvals_pending` (gauge per team), `router_approval_decision_seconds` (request → decision),
`router_audit_exported_total{sink}`, `router_audit_export_errors_total{sink}`, `router_audit_export_lag_rows{sink}`.

## 9. Testing (SysDesign §13)

- Unit (table-driven): full role × permission × scope matrix; `PolicyEngine.Evaluate` incl. disabled, workspace admin,
  cross-team read; `TeamProductionPolicy` every branch of 5.4; `SampleVisibility` matrix; approval state machine
  transitions; audit `Filter` parsing; `DeployModeResolver` (FE, Vitest) mirrors the Go table.
- Golden: `testdata/rbac/matrix.golden.json` (role → permissions) checked in; FE test asserts the mock matrix equals it.
- Integration (Testcontainers Postgres): last-admin guard under concurrent demotes; NOTIFY invalidation across two
  Manager instances; concurrent approve vs reject (exactly one wins); partial unique pending index; audit append-only
  trigger; audit query keyset paging correctness; export checkpoint resume after sink failure.
- Contract: OpenAPI paths for users, role-bindings, teams, grants, approvals, audit-log, workspace-settings.
- Coverage guard: test that every registered API route has an `EndpointRule` (fail closed).
- E2E (Playwright): admin changes editor → viewer and UI re-gates; editor requests prod deploy → approver approves →
  route active; reject with note; viewer sees masked samples; audit page filters and sensitive highlight.

## 10. Decisions

- There is one workspace per deployment. The first login becomes workspace admin. Roles are assigned by hand
  (Settings → Members). OIDC group mapping comes later (platform-core PC-79).
- Users can read other teams' routes and connections. Writes stay team-scoped.
- Masking: Viewers are always masked. Other roles are masked only on connections marked `sensitive`. Masks keep
  types. `connections.sensitive` is owned by connections, so this folder only reads it.
- Strict 4-eyes: when a team requires approval, every prod deploy needs a second person. The approver can
  never approve their own deploy, and there is no admin bypass. A rejected request still consumes its version number.
- Only workspace admins can read the audit log (`audit.read`).
- The workspace-level admin flag (`users.is_workspace_admin`) stays. It follows from the single-workspace and
  first-login-admin decisions.
- Grants: this folder owns `team_connection_grants`, `/teams/{id}/grants` and `CanUseConnection`. The
  validation rule that uses them runs in routes (`internal/routes/validation`).
- Migration `0005_access_control_core` (MVP). P2/P3 migrations take numbers ≥ 0010.
- The audit read API (`GET /audit-log`, `/audit-log/actions`) ships in MVP so the audit screen runs on the real API;
  export stays P3. Entry labels fall back to live object names at read time (no backfill migration).
- Endpoint rules stay on each feature's `RouteSpec.Permission`; coverage is enforced by `cmd/api/coverage_test.go`
  and logged at startup. Collections that filter per object (global DLQ list/summary) are allow-listed.
- Web E2E tests live in their own package `web/e2e/` (Playwright), excluded from vitest.
- P2 migrations use the folder's range 1010–1019: `1010_access_control_teams`, `1011_access_control_approval_status`,
  `1012_access_control_approvals`, `1013_access_control_invitations`.
- Invitations are claimed by a `users` BEFORE INSERT trigger (first login with the invited email takes over the pending
  row and its bindings), so OIDC and the dev seeder need no change; the Bootstrapper stamps acceptance and audits it.
- The approvals `Activator` port has two steps: `Revalidate` (before the decision transaction — routes validators run
  in parallel) and `Activate` (inside it). `deployment.approved` is audited by routes' lifecycle.
- Approvals plug into routes as its ApprovalPolicy kind `team_production` (`ROUTER_APPROVAL_POLICY`); the gate policy
  itself is selected by `ROUTER_APPROVALS_POLICY`, the notifier by `ROUTER_APPROVALS_NOTIFIER` (`log` until the WS bus).
- Routes' MVP write guard is replaced by a policy-engine guard (`route.write`), so Approvers keep every Editor right.
- Payload owners (preview, live tail, DLQ) call one port, `rbac.SampleAccess.Decide`, for "masked?" and the audit action.

- P3 migrations use the folder's range 2010–2019: `2010_access_control_workspace`, `2011_access_control_audit_search`.
- SIEM export types carry an `Export` prefix (`ExportSink`, `ExportSinkKind`, …) because platform-core's real-time audit
  stream (PC-75, best effort, drops when full) owns `audit.Sink` in the same package; the export stays a checkpointed
  at-least-once poller on the scheduler leader (`scheduler.LeaderGate`), so a slow SIEM never loses entries.
- The OPA engine lives in `internal/rbac/opa` (keeps the OPA dependency out of every rbac importer) and is registered
  by `cmd/api`; pinned to OPA v1.18.0 because later versions upgrade wazero, which js-sandbox pins.
- Audit text search resolves live names through reverse finders (ids of objects whose name matches) instead of
  naming every row; 1 M rows: p95 218 ms (was 7 s).
- Workspace settings are key/value rows; their ETag is 1 + the sum of row versions (every change adds one). New teams
  take `default_require_prod_approval`; `rbac.AIAssistGate` (`AccessWiring.Settings`) is the port transform-studio reads.

## 11. Open questions (remaining)

- Q5 Should pending approvals expire (e.g. after 7 days) and should approvers also get email, not just the bell?
