# Routes — Design (00000-init)

Scope: `internal/routes` (Go), its Postgres repositories in `internal/store`, and the FE pages
`web/src/pages/routes`, `web/src/pages/builder` (rail, Source/Filter/Destination/Review
editors) and the History + Settings tabs of `web/src/pages/route-detail`.
The config document itself (schema, Go/TS types, pure validators, filter AST, compile) is owned
by **pipeline-engine**; this folder only stores, validates in context, versions and deploys it.

## 1. Component map

```
web (builder / routes list / history)
   └─ HttpRoutesService ──HTTP /api/v1/routes──▶ cmd/api
                                                 └─ internal/routes/http   (class-based handlers)
                                                     └─ internal/routes    (Managers = business logic)
                                                         ├─ internal/store (RouteRepo, VersionRepo, DeploymentRepo)
                                                         ├─ pkg/config     (document types + pure validators)  [pipeline-engine]
                                                         ├─ ContextValidators → connections / library / test cases
                                                         ├─ ApprovalPolicyManager → access-control (P2)
                                                         ├─ RouteEventPublisher → pg NOTIFY "route_changed" → route-runtime
                                                         └─ RouteMetricsReader → monitoring
```

## 2. Go package `internal/routes`

Files are one domain concept each. All Managers are the only entry points used by handlers.

### 2.1 Domain types and enums (`domain.go`, `status.go`)

```go
type RouteID string; type VersionID string; type DeploymentID string
type Revision int64                       // = routes.row_version (platform-core 0002), sent as ETag W/"<n>"

type DesiredState string                  // mirrors route_desired_state
const ( DesiredRunning DesiredState = "running"; DesiredPaused = "paused"; DesiredArchived = "archived" )

type ObservedState string                 // mirrors route_observed_state (written by route-runtime)

type Status string                        // derived, shown in UI
const ( StatusDraft Status = "draft"; StatusRunning = "running"; StatusPaused = "paused";
        StatusError = "error"; StatusArchived = "archived" )

type VersionOrigin string                 // new enum route_version_origin
const ( OriginDeploy VersionOrigin = "deploy"; OriginRollback = "rollback"; OriginAutoRollback = "auto_rollback";
        OriginImport = "import" )

type DeploymentStatus string              // mirrors deployment_status (+ rejected, cancelled from access-control P2)
const ( DeployPendingApproval DeploymentStatus = "pending_approval"; DeployRollingOut = "rolling_out";
        DeployActive = "active"; DeployFailed = "failed"; DeploySuperseded = "superseded" )

type TransformIndicator string            // list column
const ( TransformNone TransformIndicator = "none"; TransformVisual = "visual"; TransformJS = "js" )

type Route struct {
    ID RouteID; TeamID string; Name, Description string; Tags []string
    DesiredState DesiredState; ActiveVersionID *VersionID; Replicas int
    Draft *config.RouteDocument; Revision Revision
    CreatedBy, UpdatedBy string; CreatedAt, UpdatedAt time.Time
}
type RouteVersion struct {
    ID VersionID; RouteID RouteID; No int; Config config.RouteDocument; ConfigHash string
    Origin VersionOrigin; BasedOn *VersionID; CreatedBy string; ChangeNote string; CreatedAt time.Time
}
type Deployment struct {
    ID DeploymentID; RouteID RouteID; VersionID VersionID; PreviousVersionID *VersionID
    Status DeploymentStatus; FailureReason string; RequestedBy, ApprovedBy string
    RequestedAt time.Time; ApprovedAt, StartedAt, FinishedAt *time.Time
}
```

`RouteStatusResolver` (class) derives `Status` from `(desired_state, active_version_id, observed_state)`:

```
resolve(route, runtime)
 └─ desired = archived                       → archived
 └─ active_version_id is null                → draft
 └─ runtime.observed = error                 → error
 └─ desired = paused                         → paused
 └─ otherwise                                → running
```

### 2.2 Managers (business logic)

| Class | Responsibility |
| --- | --- |
| `RouteManager` | create, get, list (summaries), save draft, rename/settings, duplicate, archive |
| `StateManager` | pause, resume (desired state) + publish change |
| `ValidationManager` | runs a `ValidatorSet` per `ValidationMode` (draft / deploy / rollback / import) |
| `DeployManager` | deploy, rollback; creates version + deployment; asks `ApprovalPolicyManager` |
| `DeploymentLifecycle` | deployment state machine; `Activate`, `MarkActive`, `MarkFailed` (auto-rollback = new version, origin `auto_rollback`) |
| `VersionManager` | list/get versions, diff (delegates to `RouteDiffer`) |
| `PortabilityManager` | export / import; JSON↔YAML via pipeline-engine `pkg/config` `DocumentFormat`; connection id ↔ name resolution here |
| `BulkActionManager` | dispatches to `BulkActionHandler` registry (pause, resume, add_tag, export) |
| `ReplayManager` (P3) | owns `POST /routes/{id}/replay` + status GET; validates request, delegates offset reset to route-runtime `OffsetResetter` |

### 2.3 Extensible domains (base interface + Manager + config-selected impls)

```go
// Doc-only rules live in pkg/config (pipeline-engine); every external-data rule lives here
// (internal/routes/validation), behind ports defined here: ConnectionLookup (connections),
// TopicInspector (connections pkg/kafka Admin), SampleSchemaProvider (live-preview),
// TestCaseRunner (transform-studio), LibraryLookup (transform-studio), GrantChecker (access-control).
type ValidationMode string
const ( ModeDraft ValidationMode = "draft"; ModeDeploy = "deploy"; ModeRollback = "rollback"; ModeImport = "import" )

type ContextValidator interface {
    Name() ValidatorName
    Modes() []ValidationMode
    Validate(ctx context.Context, in ValidationInput) []config.Issue
}
// impls: DocumentValidator (wraps pkg/config doc-only profile: schema, delivery mode, compile),
//        ConnectionAccessValidator, DestinationTopicValidator, FieldTypeValidator (warnings),
//        UniqueNameValidator, TestCaseValidator (P2; error on deploy, warning on draft/rollback),
//        LibraryRefValidator (P3; missing → error, newer version → LibraryUpdateAvailable warning)
type ValidatorSet struct{ validators []ContextValidator }      // registered in wiring, filtered by Modes()

type ApprovalPolicy interface {                                 // selected by ROUTER_APPROVAL_POLICY
    Kind() ApprovalPolicyKind                                   // "none" (MVP) | "production" (P2)
    Requires(ctx context.Context, route Route, doc config.RouteDocument) (bool, error)
}

type DocumentCodec interface {                                  // selected by ?format=; thin adapter over
    Format() DocumentFormat                                     // pkg/config DocumentFormat ("json" | "yaml")
    Encode(docs []config.RouteDocument) ([]byte, error)         // multi-doc for bulk export
    Decode(raw []byte) ([]config.RouteDocument, error)
}

type DiffSection interface {                                    // source, filter, transform, destination, settings
    Section() DiffSectionKind
    Diff(a, b config.RouteDocument) []DiffLine
}

type BulkActionHandler interface {
    Action() BulkAction                                         // pause | resume | add_tag | export
    Apply(ctx context.Context, ids []RouteID, args BulkArgs) BulkResult
}

type RouteEventPublisher interface {                            // impl: PgNotifyPublisher
    Publish(ctx context.Context, ev RouteChanged) error         // channel "route_changed"
}
type RouteMetricsReader interface {                             // impl from monitoring; NullMetricsReader until ready
    Summaries(ctx context.Context, ids []RouteID) (map[RouteID]MetricsSummary, error)
}
type TemplateKind string                                        // FE-only today; reserved for server templates
```

Domain helpers (classes, no free helper functions):
- `ConfigFingerprint` — canonical JSON (sorted keys) → `sha256` hex for `config_hash`.
- `FilterSummarizer` — AST → `{headline:"eventType is OrderPaid", more:2}` for the list.
- `TransformClassifier` — steps → `TransformIndicator` (any `js` step → js; any step → visual).
- `DuplicateNamer` — `"<name>-copy"`, `"<name>-copy-2"` … unique per team.
- `ConsumerGroupNamer` — `router.<route-name>` default when the draft omits it.

### 2.4 HTTP handlers (`internal/routes/http`)

One struct per resource group implementing `Mount(chi.Router)`: `RoutesHandler`, `DraftHandler`,
`LifecycleHandler` (validate/deploy/pause/resume/rollback/replay), `VersionsHandler`,
`PortabilityHandler`, `BulkHandler`. Path params, query keys and header names are enum types
(`PathParam`, `QueryKey`, `HeaderName`). Errors map to `problem.Details` through one
`RoutesProblemMapper` (domain error → status + type + `path`).

### 2.5 Repositories (`internal/store`, Postgres implementations)

```go
type RouteRepository interface {
    store.Repository[routes.Route, routes.RouteID]
    ListSummaries(ctx, q routes.ListQuery, page store.Page) ([]routes.SummaryRow, string, error)
    SaveDraft(ctx, id routes.RouteID, doc config.RouteDocument, expected routes.Revision) (routes.Route, error) // ErrRevisionMismatch
    SetDesiredState(ctx, ids []routes.RouteID, s routes.DesiredState) ([]routes.Route, error)
    SetActiveVersion(ctx, id routes.RouteID, v *routes.VersionID) error
    AddTag(ctx, ids []routes.RouteID, tag string) error
}
type VersionRepository interface {
    Create(ctx, v routes.RouteVersion) (routes.RouteVersion, error)  // version_no = max+1 under row lock on routes
    List(ctx, id routes.RouteID, page store.Page) ([]routes.RouteVersion, string, error)
    GetByNo(ctx, id routes.RouteID, no int) (routes.RouteVersion, error)
}
type DeploymentRepository interface {
    Create(ctx, d routes.Deployment) (routes.Deployment, error)
    SupersedeOpen(ctx, id routes.RouteID) error                      // pending_approval|rolling_out → superseded
    Transition(ctx, id routes.DeploymentID, from, to routes.DeploymentStatus, reason string) error
    Latest(ctx, id routes.RouteID) (routes.Deployment, error)
}
```
All multi-row writes run inside `store.TxManager.WithinTx`.

## 3. Data model touchpoints

Reads/writes `routes`, `route_versions`, `deployments`; reads `route_runtime`, `connections`,
`teams`, `test_cases`. Migration `0007_routes_lifecycle` (fixed MVP number). The `If-Match` token
is `routes.row_version` from platform-core `0002` (no own revision column):

| Change | Why |
| --- | --- |
| `routes.description text`, `routes.created_by uuid`, `routes.updated_by uuid` | list "Last changed" by whom, builder description |
| `CREATE TYPE route_version_origin AS ENUM ('deploy','rollback','auto_rollback','import')`; `route_versions.origin` NOT NULL DEFAULT 'deploy' | history labels |
| `route_versions.based_on_version_id uuid REFERENCES route_versions(id)` | rollback provenance |
| `deployments.previous_version_id uuid REFERENCES route_versions(id)` | auto-rollback source (copied into a new version) |
| `deployments.failure_reason text` | shown in History / Review |
| index `routes (team_id, updated_at DESC, id)` | list cursor order |
| index `route_versions (route_id, version_no DESC)` already covered by UNIQUE | — |

Cursor for the list = base64 of `(updated_at, id)`; versions cursor = `version_no`.

## 4. API (owned endpoints, all under `/api/v1`)

Common: auth from platform-core; RBAC check per endpoint via access-control `Authorizer`
(MVP: admin + editor of the owning team may mutate; any user may read routes of every team); every mutation emits an
audit entry through the platform-core audit middleware with `object_type="route"`.

| Method & path | Phase | Request | Response |
| --- | --- | --- | --- |
| `GET /routes` | MVP | `?q&status&connectionId&tag&teamId&cursor&limit` | `200 {items: RouteSummary[], nextCursor}` |
| `POST /routes` | MVP | `{name, teamId?, description?, tags?, draft?: RouteDocument}` | `201 RouteDetail` + `ETag` |
| `GET /routes/{id}` | MVP | — | `200 RouteDetail` + `ETag` |
| `PATCH /routes/{id}` | MVP | `If-Match`; `{name?, description?, tags?, replicas?, draft?}` | `200 RouteDetail` (+ `issues` from draft mode) |
| `DELETE /routes/{id}` | MVP | — | `204` (archive) |
| `POST /routes/{id}/duplicate` | MVP | `{name?}` | `201 RouteDetail` |
| `POST /routes/{id}/validate` | MVP | `{draft?: RouteDocument, mode?: "draft"|"deploy"}` | `200 {valid, issues: Issue[]}` |
| `POST /routes/{id}/deploy` | MVP | `If-Match`; `{changeNote?}` | `202 {deployment, version, route}` or `422` issues |
| `POST /routes/{id}/pause` · `/resume` | MVP | — | `200 RouteDetail` |
| `GET /routes/{id}/versions` | MVP | `?cursor&limit` | `200 {items: VersionSummary[], nextCursor}` |
| `GET /routes/{id}/versions/{no}` | MVP | — | `200 RouteVersion` (full config) |
| `GET /routes/{id}/versions/{a}/diff/{b}` | MVP | `a,b` = number, `active` or `draft` | `200 RouteDiff` |
| `POST /routes/{id}/rollback` | MVP | `{version, changeNote?}` | `202 {deployment, version, route}` |
| `GET /routes/{id}/deployments` | MVP | `?cursor&limit` | `200 {items: Deployment[], nextCursor}` |
| `POST /routes/bulk` | MVP | `{action: "pause"|"resume"|"add_tag", ids, tag?}` | `200 {results: [{id, ok, problem?}]}` |
| `GET /routes/{id}/export` | P2 | `?format=yaml|json&version=active|draft|N` | `200` document (`application/yaml`) |
| `POST /routes/export` | P2 | `{ids, format}` | `200` multi-doc YAML |
| `POST /routes/import` | P2 | body YAML/JSON; `?mode=create|upsert&dryRun=bool` | `200 {results: [{name, routeId?, action, issues}]}` |
| `POST /routes/{id}/replay` | P3 | `{from: RFC3339, confirm: true}` | `202 {replayId}` (mechanism: route-runtime; dialog: monitoring) |
| `GET /routes/{id}/replays/{replayId}` | P3 | — | `200 {status, from, startedAt, finishedAt?, error?}` |

Shapes:
```jsonc
// RouteSummary
{ "id":"…", "name":"order-paid-to-partner", "teamId":"…", "teamName":"payments", "tags":["pii"],
  "status":"running", "desiredState":"running", "observedState":"running",
  "activeVersion":{"id":"…","no":4} , "hasUndeployedDraft":true,
  "source":{"connectionId":"…","connectionName":"orders-prod","topics":["order-events"],"topicPattern":null},
  "filterSummary":{"headline":"eventType is OrderPaid","more":2},
  "transform":"js",
  "destinations":[{"branchId":"main","connectionId":"…","connectionName":"partner","topic":"orders-public"}],
  "metrics":{"eventsPerSec":120.5,"matchPct":7.4,"lag":12,"sparkline":[…]} | null,
  "updatedAt":"…", "updatedBy":{"id":"…","name":"Quincy"} }
// RouteDetail = RouteSummary + { description, replicas, revision, draft: RouteDocument|null,
//                                active: RouteDocument|null, lastDeployment: Deployment|null }
// Issue  = { path:"/branches/0/filter/children/1/value", message, severity:"error"|"warning", rule }
// VersionSummary = { no, origin, basedOn?:number, author, changeNote, createdAt, configHash, deployment: {status} }
// RouteDiff = { a, b, sections:[{section:"filter", lines:[{mark:"+"|"-"|"~", text, path}]}] }
```
Error mapping: `412` stale `If-Match` (type `conflict`), `428` missing `If-Match`, `409` deploy on
archived route / resume a draft / name taken, `422` validation errors with `path` of the first
error and the full `issues` list as an extension member, `404` unknown route/version.

## 5. Flows

### 5.1 List routes
```
HTTP GET /api/v1/routes?q&status&cursor
 └─ RoutesHandler.list → RouteManager.List(query, page)
     └─ RouteRepository.ListSummaries (join route_runtime, route_versions active, connections)
         └─ rows → RouteStatusResolver.resolve each → status
         └─ FilterSummarizer + TransformClassifier on active (or draft) config
     └─ RouteMetricsReader.Summaries(ids)
         └─ reader unavailable → metrics = null (list still renders, warn logged)
     └─ 200 {items, nextCursor}
```
---
### 5.2 Create route from a template (FE)
```
User clicks "+ New route" (or presses N)
 └─ NewRouteChooser shows 4 RouteTemplate cards
     └─ user picks template → RouteTemplate.apply(emptyDraft) → builder opens with unsaved draft (no id)
         └─ first Save draft → POST /routes {name, draft} → 201 → URL becomes /routes/{id}/edit
             └─ name taken → 409 → inline error on name field
```
---
### 5.3 Save draft
```
User presses Save draft / Ctrl+S
 └─ RouteDraftMapper.toDocument(builder draft) → PATCH /routes/{id} If-Match: W/"7"
     └─ DraftHandler.patch → RouteManager.SaveDraft
         └─ document not parseable as RouteDocument → 422 {path} → field highlighted
         └─ ValidationManager.Run(ModeDraft) → issues (never blocks save)
         └─ RouteRepository.SaveDraft(expected=7)
             └─ revision mismatch → 412 → FE shows "changed by someone else, reload / overwrite"
             └─ ok → revision 8, updated_by set → audit "route.draft_saved" (diff)
         └─ 200 RouteDetail + ETag W/"8" + issues → rail cards show tick / warning, dots cleared
```
---
### 5.4 Validate
```
HTTP POST /routes/{id}/validate {draft?, mode}
 └─ LifecycleHandler.validate → ValidationManager.Run(mode, draft ?? stored draft)
     └─ ValidatorSet filtered by mode, each validator runs [parallel, bounded, ctx timeout 5 s]
         └─ DocumentValidator (pkg/config doc-only)     → schema / exactly_once / compile errors, JSON Pointer paths
         └─ ConnectionAccessValidator                   → unknown / not granted to team (P2 grants) → error
         └─ DestinationTopicValidator (pkg/kafka Admin) → missing and no createTopic → error
         └─ FieldTypeValidator (live-preview schema)    → unknown field / op-type mismatch → warning only
         └─ LibraryRefValidator [P3]                    → unknown version → error; newer version → warning
         └─ TestCaseValidator [P2]                      → failing case → error (deploy) / warning (draft, rollback)
         └─ a validator times out                       → warning "check skipped: <name>"
     └─ 200 {valid = no error issues, issues sorted by path}
```
---
### 5.5 Deploy
```
HTTP POST /routes/{id}/deploy If-Match {changeNote}
 └─ LifecycleHandler.deploy → DeployManager.Deploy
     └─ route archived → 409
     └─ no draft or draft hash = active hash → 409 "nothing to deploy"
     └─ ValidationManager.Run(ModeDeploy)
         └─ any error issue → 422 {issues}, nothing written
     └─ ApprovalPolicyManager.Requires(route, doc)
     └─ TxManager.WithinTx
         └─ lock routes row, check revision (412 on mismatch)
         └─ DeploymentRepository.SupersedeOpen(route) → older pending/rolling_out → superseded
         └─ VersionRepository.Create(no = max+1, hash, origin=deploy)
         └─ approval required → Deployment{pending_approval}; active version unchanged
         └─ no approval → Deployment{rolling_out, previous_version_id = current active}
             → routes.active_version_id = new, desired_state = running
     └─ after commit, no approval: RouteEventPublisher.Publish(route_changed{id, version}) [event]
         → route-runtime reconcilers roll out; monitoring relays to WS /events
     └─ after commit, approval: ApprovalRequester.RequestApproval(deployment) (access-control approvals.Request)
         → older pending requests cancelled, approvers notified (approval.requested)
     └─ audit "route.deploy_requested" | "route.deployed"
     └─ 202 {deployment, version, route}
```
---
### 5.6 Approval decision (P2, called by access-control)
```
access-control ApprovalsService approve(deploymentId)
 └─ DeploymentLifecycle.Activate(deploymentId, approver)
     └─ status ≠ pending_approval (superseded meanwhile) → 409, approval marked stale
     └─ TxManager.WithinTx
         └─ Transition pending_approval → rolling_out, approved_by/at set, previous_version_id = current active
         └─ routes.active_version_id = deployment.version_id, desired_state = running
     └─ RouteEventPublisher.Publish [event] → route-runtime rollout
access-control reject(deploymentId)
 └─ DeploymentLifecycle.Reject → Transition pending_approval → rejected (enum value from access-control
    approvals migration); the version row stays, so its number is consumed
```
---
### 5.7 Rollout outcome (called in-process by route-runtime rollout tracker)
```
[event] all replicas report version V running (route-runtime)
 └─ DeploymentLifecycle.MarkActive(deploymentId)
     └─ Transition rolling_out → active, finished_at → audit "route.rollout_active"
```
---
```
[event] replica reports V errored within 60 s of started_at (route-runtime)
 └─ DeploymentLifecycle.MarkFailed(deploymentId, reason)
     └─ Transition rolling_out → failed, failure_reason
     └─ previous_version_id present → new Version{origin=auto_rollback, based_on=previous, config=previous.config}
         → new Deployment{rolling_out} → routes.active_version_id = new → Publish route_changed [event]
         → workers return to the previous config as v(max+1); WS bell "Deploy v5 failed, rolled back as v6 (= v4)"
     └─ no previous version (first deploy) → desired_state = paused → Publish route_changed
```
---
### 5.8 Pause / resume
```
HTTP POST /routes/{id}/pause
 └─ StateManager.Pause
     └─ desired = archived → 409
     └─ no active version → 409 "draft routes are not running"
     └─ desired_state = paused → audit → Publish route_changed [event] → runners stop fetching
     └─ 200 RouteDetail (status paused)
```
---
```
HTTP POST /routes/{id}/resume
 └─ StateManager.Resume
     └─ no active version → 409 "deploy first"
     └─ desired_state = running → audit → Publish route_changed [event]
```
---
### 5.9 Archive
```
HTTP DELETE /routes/{id}
 └─ RouteManager.Archive
     └─ already archived → 204 (idempotent)
     └─ desired_state = archived, open deployments superseded → audit → Publish route_changed [event]
         → runners stopped; route hidden from default list (status filter "archived" shows it)
```
---
### 5.10 Rollback
```
HTTP POST /routes/{id}/rollback {version: N, changeNote}
 └─ DeployManager.Rollback
     └─ version N unknown → 404; N = active → 409 "already active"
     └─ ValidationManager.Run(ModeRollback, version N config)
         └─ connection deleted / library missing → 422 (old config no longer valid)
         └─ failing test cases → warning only
     └─ same transaction as Deploy with Version{origin=rollback, based_on=N, config = N.config}
     └─ draft replaced by N.config (builder shows what is live)
     └─ 202 {deployment, version: v(max+1), route}
```
---
### 5.11 Diff
```
HTTP GET /routes/{id}/versions/{a}/diff/{b}
 └─ VersionsHandler.diff → VersionManager.Diff(refA, refB)
     └─ ref "draft" → routes.draft_config; "active" → active version; number → version row
     └─ ref missing (e.g. no active) → empty side, all lines "+"
     └─ RouteDiffer runs each DiffSection (source, filter, transform, destination, settings)
         └─ filter section renders cond rows as readable text ("amount > 100") via FilterSummarizer
         └─ transform section aligns steps by index + kind, "~" for changed params
     └─ 200 RouteDiff
```
---
### 5.12 Export / import (P2)
```
HTTP GET /routes/{id}/export?format=yaml&version=active
 └─ PortabilityManager.Export → DocumentCodecManager.codec(format)
     └─ unknown format → 400
     └─ strip server-only fields (ids of team) ; connections exported by name, not id
     └─ 200 application/yaml (Content-Disposition attachment)
```
---
```
HTTP POST /routes/import?mode=upsert&dryRun=false (YAML, may be multi-doc)
 └─ PortabilityManager.Import → codec.Decode
     └─ decode error → 400 problem with line/column in detail
     └─ per document:
         └─ resolve connection names → ids; unknown → issue (error)
         └─ ValidationManager.Run(ModeImport) → issues
         └─ name exists & mode=create → result action "skipped_exists"
         └─ name exists & mode=upsert → SaveDraft (no deploy) → action "draft_updated"
         └─ new name → Create → action "created"
     └─ dryRun → no writes, same result list
     └─ 200 {results}
```
---
### 5.13 Bulk action
```
HTTP POST /routes/bulk {action, ids, tag?}
 └─ BulkActionManager.Run → registry[action]
     └─ unknown action → 400
     └─ > 200 ids → 422
     └─ handler applies per id (each its own tx, RBAC checked per route)
         └─ per-id failure → result {ok:false, problem}; others continue
     └─ 200 {results}
```
---
### 5.14 Replay (P3)
```
HTTP POST /routes/{id}/replay {from, confirm:true}
 └─ ReplayManager.Request
     └─ confirm missing → 422 "destination will receive duplicates"
     └─ from in future or before retention → 422
     └─ OffsetResetter.Reset(route, from) (route-runtime ReplayCoordinator) [async] → audit "route.replay"
     └─ 202 {replayId}; progress pushed on WS /events; GET /routes/{id}/replays/{replayId} for status
     (confirm dialog in the route-detail header is owned by monitoring)
```
---
### 5.15 Builder editing (FE)
```
User opens /routes/{id}/edit
 └─ RoutesService.get(id) → RouteDraftMapper.fromDocument(draft ?? active) → BuilderStore
     └─ live-preview SamplePanel loads samples for source (session id kept in store)
```
---
```
User edits a step
 └─ BuilderStore marks card dirty (dot) → debounced 400 ms → live-preview run → match counter, statuses
```
---
```
User right-clicks value in sample panel → "Keep only events where field = value"
 └─ RouteActions.addFilterRow(field, eq, value) → filter editor selected, new row highlighted
```
---
```
User selects a filtered sample event
 └─ trace per-node results → ConditionRow tone ok/err (why filtered)
```
---
```
User toggles Expression mode
 └─ FilterExpressionBuilder.toExpression(group) → expr node
```
---
```
User toggles back
 └─ ExpressionParser.toGroup(expr) succeeds → visual rows
 └─ not simple enough → stay in expression mode, notice "can't convert this expression"
```
---
```
User clicks Deploy
 └─ unsaved changes → Save draft first (5.3)
 └─ Review step: validate (5.4) + diff active↔draft (5.11) + dry run (live-preview)
     └─ errors → Deploy disabled, issues list links to step + field
     └─ DeployDialog(changeNote) → POST deploy → toast + navigate to route detail
         └─ 202 pending_approval → button label "Request approval", toast "approval requested"
```

## 6. Frontend design

Existing: `web/src/domain/route.ts`, `builder.ts` (RouteDraft, RouteTemplate classes),
`filter.ts`, `version.ts`, `pipeline/FilterExpressionBuilder.ts`, `api/contracts.ts`
(`RoutesService` abstract class), `api/mock/MockRoutesService.ts`, `store/RouteActions.ts`.

Changes:
- `RoutesService` gains `validate`, `diff`, `listDeployments`, `bulk`, `exportRoute`, `importRoutes`,
  `archive`, `create`; `HttpRoutesService` (in `web/src/api/http/`) implements it against `/api/v1`
  using the platform-core generated client; mock keeps parity; selection via `api/config.ts`.
- `RouteDraftMapper` (`web/src/domain/routeConfig/RouteDraftMapper.ts`): builder `RouteDraft` ↔
  generated `RouteDocument` (pipeline-engine TS types). Display labels stay in UI enums only.
- `FilterTreeModel` replaces the flat `FilterGroup.rows` with nested groups mapping 1:1 to the AST
  (`and|or|not|cond|expr`); `OperatorCatalog` maps field type → allowed operators (all engine ops).
- Status badge uses `RouteStatusMeta`; derived status comes from the API.

Pages / components:

| Path | Components |
| --- | --- |
| `pages/routes/` | `RoutesPage`, `RoutesToolbar`, `RoutesTable`, `RouteCardGrid` (P2), `BulkActionBar`, `NewRouteChooser`, `RoutesEmptyState`, `ImportRoutesDialog` (P2) |
| `pages/builder/` | `BuilderPage` (3-zone layout), `BuilderTopBar`, `PipelineRail`, `RailCard`, `BranchRail` (P2) |
| `pages/builder/steps/` | `SourceStepEditor`, `FilterStepEditor`, `DestinationStepEditor`, `ReviewStep`, `DeployDialog` |
| `pages/builder/filter/` | `ConditionGroup`, `ConditionRow`, `FieldPicker`, `OperatorSelect`, `ValueInput`, `ExpressionEditor`, `MatchCounter` |
| `pages/route-detail/` | `RouteDetailPage` (header: name, status, Pause/Resume, Edit, `HeaderActionSlot`; `RouteDetailTabRegistry`), `HistoryTab`, `VersionList`, `VersionDiffView`, `SettingsTab` (hosts monitoring's `AlertRulesPanel`, P2) |

Embedded from other folders: `SamplePanel` (live-preview), `TransformStudio` (transform-studio),
`TopicPicker` + `CreateTopicDialog` (connections), Overview/Live tail tabs + `ReplayDialog` in the header slot
(monitoring), Dead letters tab (dead-letters).

## 7. Error cases (summary)

| Case | Result |
| --- | --- |
| Concurrent edit | 412, FE offers reload or overwrite (re-PATCH with fresh ETag) |
| Deploy with errors | 422 + issues; nothing written |
| Deploy with no changes | 409 "nothing to deploy" |
| Deploy while another is pending/rolling out | older one superseded, new proceeds |
| Approval of superseded deployment | 409, approval stale |
| Rollout fails ≤ 60 s | failed + auto-rollback as a new version copying the previous one |
| Connection deleted after deploy | route keeps running (runtime); next validate errors on that path |
| Resume / pause a draft | 409 |
| Metrics unavailable | list shows "—" for throughput/match/lag |
| Validator dependency down (Kafka list topics) | warning "check skipped", deploy allowed |
| Import of unknown connection name | per-doc error issue, others continue |

## 8. Decisions

1. Save draft always saves any parseable draft and lists issues. Deploy is blocked by errors.
2. A version row is created when approval is requested, so a rejected approval still uses up its version number.
3. Rollback treats failing test cases as warnings.
4. Archive is the only delete. Unarchive comes in P2 (`routes#p2-be-unarchive`).
5. Replay: this folder owns `POST /routes/{id}/replay` (+ status GET). route-runtime owns the offset
   reset mechanism (`ReplayCoordinator`, `route_replays`). monitoring owns the confirm dialog in the
   route-detail header. This folder owns the header shell and its action slot.
6. Filter Code mode reuses the `expr` node. Filter expression mode is MVP.
7. Export uses connection names. Import resolves names within the team.
8. Auto-rollback (route-runtime trigger) creates a new version (`origin=auto_rollback`), just like a manual rollback.
9. External-data validation rules live in `internal/routes/validation`. Doc-only rules stay in `pkg/config`.
   Issue paths are JSON Pointers.
10. The If-Match token is platform-core's `routes.row_version`. Migration `0007_routes_lifecycle`.
11. Users can read other teams' routes. Paused routes stop their runners.
12. YAML/JSON conversion is pipeline-engine's `DocumentFormat`. This folder adds only connection id↔name resolution.
13. P2 approvals: `ROUTER_APPROVAL_POLICY` defaults to `auto` = access-control's production gate (`team_production`)
    when it is wired, else `none`. Routes registers each pending deployment through the `ApprovalRequester` port.
14. Export keeps the schema shape: the `connectionId` members hold connection names; import maps names back to ids
    (team connections first, existing ids kept, anything else is a per-document issue). The default consumer group
    is left out of exports so an imported copy gets its own. Browsers send imports as `{content, format}`.
15. Rollout notices (auto-rollback / paused after a failed rollout) go through `RolloutNotifier` (default `log`);
    monitoring plugs in the WS bell once its event bus exists.
16. Source shape rules the schema cannot express (topics xor RE2 pattern, offsets per partition, timestamp needs
    `at`) and the schema-registry check for Avro/Protobuf live in `internal/routes/validation` until pkg/config has them.
17. Builder branches: the edited branch stays in the draft's filter/steps/destination and the others wait in
    `RouteDraft.branching`, so every step editor is unchanged. Filter Code mode is stored as the expr
    `(function filter(event, ctx) {…})(event, ctx)`.
