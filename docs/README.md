# Event Router — Documentation Index

Each folder holds one active version (the highest numbered folder, usually `00000-init`), which contains `01-requirements.md` (≤ 100 lines),
`02-design.md` (with a "Decisions" section) and `03-plan.md` (checklist only). Plan items have ids, and other
folders refer to them as `needs: <folder>#<id>`.

## Folders

| Folder | Item ids | Purpose |
| --- | --- | --- |
| [systems/platform-core](systems/platform-core/00000-init/) | `PC-xx` | API base and conventions (RFC 7807, JSON Pointer paths, cursor, If-Match), Postgres + migrations, OIDC/dev auth, secrets (Vault/local/KMS), audit write, deploy, generated web client |
| [systems/pipeline-engine](systems/pipeline-engine/00000-init/) | `PE-xx` | Route config JSON Schema + generated Go/TS types, doc-only validation, `pkg/engine` (codecs, filter, recipes, trace), `routerctl`, static engine catalogue |
| [systems/js-sandbox](systems/js-sandbox/00000-init/) | `JS-xx` | QuickJS-on-wazero sandbox for JS steps and `expr` filters: limits, host `ctx` helpers, determinism, pools |
| [systems/route-runtime](systems/route-runtime/00001-stress-test/) | `RR-xx`, `RR-ST-xx`, `RR-OPT-xx` | Worker runners (at-least-once, DLQ write, backpressure), scheduler/leases, hot reload, rollout + auto-rollback detection, replay mechanism. Versions: `00000-init` (base), `00001-stress-test` (active: stress-test results, limits, follow-up optimisations) |
| [features/connections](features/connections/00000-init/) | `CN-xx` | Kafka connections: CRUD, security modes, write-only secrets, live test checklist, health probe, topic browser, create topic, `connections.*` columns |
| [features/routes](features/routes/00000-init/) | `routes#<slug>` | Routes list, 3-zone builder, external-data validation, deploy/versions/diff/rollback, route-detail shell, YAML import/export, replay API |
| [features/live-preview](features/live-preview/00000-init/) | `LP-xx` | Sample sessions (latest/time range/upload), preview run with trace, sample panel, masking, Peek backend |
| [features/access-control](features/access-control/00000-init/) | `AC-xx` | RBAC roles/permissions, users & role bindings, teams + connection grants, approvals (4-eyes), masking decision, audit log read/export |
| [features/dead-letters](features/dead-letters/00000-init/) | `DL-xx` | DLQ browse (route + global), detail, retry (in-API engine), discard, export, Open in Studio |
| [features/monitoring](features/monitoring/00000-init/) | `M-BE/M-FE/M-T-xx`, `P2-*`, `P3-*` | Dashboard, route overview metrics, alerts + bell, WS events, flow map, live tail, demo data, replay confirm dialog |
| [features/transform-studio](features/transform-studio/00000-init/) | `TS-xx` | Transform Studio (recipes, JS editor, click-to-transform), saved test cases, visual mapper, library, AI assist (mock) |

## Local development

Go services (`api`, `worker`, `preview`) and the vite dev server run on your machine against the staging
backing services (Postgres DB `router_dev`, Redis DB 1, shared Kafka, Prometheus) through an ssh tunnel:
`deploy/dev/dev.sh up|down|status|logs <svc>`. Details: [deploy/dev/README.md](../deploy/dev/README.md).

## Build waves

1. **Wave 1:** pipeline-engine, js-sandbox, platform-core (MVP items).
2. **Wave 2:** connections, routes, live-preview, access-control (MVP items).
3. **Wave 3:** route-runtime, dead-letters, monitoring, transform-studio BE (MVP items).
4. **Then P2** items of all folders.
5. **Then P3** items of all folders.

Any `needs:` that points to a later wave or phase means the item waits for that dependency or uses a fake or mock
until it lands.

## Fixed migration order (MVP)

| # | Migration | Owner |
| --- | --- | --- |
| 0001 | `0001_init` (existing) | — |
| 0002 | `0002_row_version` | platform-core |
| 0003 | `0003_secrets_audit` (secrets + audit_log request_id/team_id/actor_kind) | platform-core |
| 0004 | `0004_connections` (auth_failed, sensitive, allow_topic_create, probe_enabled, kind, …) | connections |
| 0005 | `0005_access_control_core` (workspace admin, users.disabled_at, audit_log.sensitive/object_label) | access-control |
| 0006 | `0006_team_hash_salt` | js-sandbox |
| 0007 | `0007_routes_lifecycle` (incl. `route_version_origin` with `auto_rollback`) | routes |
| 0008 | `0008_route_runtime` (incl. `router_runtime_notify` trigger, `deployments.activated_at`) | route-runtime |

0009 is unassigned. P2/P3 migrations start at **0010** and take the next free number at implementation time.

## Decisions

### User decisions (final)
- There is one workspace per deployment.
- Roles are assigned by hand, and the first login becomes admin. OIDC group mapping comes later (platform-core PC-79, P3).
- Secrets use Vault Transit in MVP and a local key-file provider for dev. AWS/GCP KMS adapters come in P2.
- Approvals are strict 4-eyes: an approver can never approve their own deploy.
- Users can READ other teams' routes and connections. Writes stay team-scoped.
- Masking: Viewers are always masked. Other roles are masked only on connections marked `sensitive`.
- Only workspace admins can read the audit log.
- AI uses a provider-agnostic `LLMProvider` interface plus a mock provider.
- Auth is generic OIDC plus a dev login.
- The route builder is an editor only (3-zone editor, no wizard).
- Scope covers all phases (MVP, P2, P3).

### Cross-folder decisions (orchestrator)
- **Replay:** routes owns the API `POST /routes/{id}/replay` (+ `GET …/replays/{replayId}`). route-runtime owns
  the offset reset (`ReplayCoordinator`, `route_replays`). monitoring owns the confirm dialog in the
  route-detail header. routes owns the header shell and its action slot.
- pipeline-engine adds `errorPolicy.dlqConnectionId` to the route schema. When it is absent, the source connection is used.
- connections owns the `connections.sensitive` column and toggle. Other folders only read it.
- Issue and problem paths use JSON Pointer.
- Types are generated from the JSON Schema (go-jsonschema / json-schema-to-typescript). The engine catalogue
  ships as static generated JSON.
- Filter `expr` mode is MVP. Code mode (P2) reuses `expr`.
- Each JS call gets a fresh context. `ctx.now` is frozen per event (processing time). The hash salt is per team.
- We build our own QuickJS-ng wasm only if a wasi-sdk toolchain is available. Otherwise we use an existing
  Go-embeddable QuickJS wasm module.
- Auto-rollback creates a new version (origin `auto_rollback`), like a manual rollback.
- Paused routes stop their runners.
- DLQ retry runs as an engine call inside the API in MVP. A retry that fails again is only reported. DLQ retention is 7 days.
- Save draft always saves and lists issues. Deploy is blocked by errors.
- A rejected approval still consumes its version number.
- Rollback treats failing test cases as warnings.
- Archive is the only delete. Unarchive comes in P2.
- YAML export uses connection names. JSON↔YAML conversion is pipeline-engine's `DocumentFormat`.
- Doc-only rules live in `pkg/config`. External-data rules live in `internal/routes/validation`.
- Click-to-transform is MVP. Inline lookup tables are MVP. Named tables come in P3.
- Test cases run in `cmd/api`.
- The library is open read.
- Explain covers recipe chains (rule-based) and JS steps (LLM).
- Below 1440 px, the Studio output column and the sample panel share the right zone as tabs.
- Sample session TTL slides.
- Masking keeps types (strings → masked string, numbers → 0).
- Uploads are allowed without a source connection.
- Writing to a test topic is P2.
- Peek reuses `POST /preview/samples` with `count=20`.
- OAuth security mode is P2.
- Topic creation is opt-in per connection.
- The health check runs every 60 s, with a per-connection opt-out.
- The alert rule form lives in the route-detail Settings tab.
- SMTP settings come from env config.
- Demo data uses an in-memory fake connection kind (`kind=demo`).

### Ownership fixes applied (one owner per endpoint / column / migration)
- `routes.row_version` (platform-core 0002) is the only If-Match token for routes. routes does not add its own `revision` column.
- routes 0007 owns `deployments.previous_version_id` and `failure_reason`. route-runtime 0008 adds only `activated_at`.
- `users.disabled_at` is owned by access-control 0005. The platform-core P2 migration adds only `last_login_at`.
- Connection grants: access-control owns `team_connection_grants`, `/teams/{id}/grants` and `CanUseConnection`.
  connections has no grants table or endpoint. routes runs the validation rule.
- live-preview owns the `pkg/kafka` sampler and serves Peek. connections has no peek endpoint.
- The deployment `rejected` / `cancelled` status values come from access-control's approvals migration (P2).
- Live tail: monitoring owns the `Tap`/`RedisTap` (flag, rate limit, sink). route-runtime owns only the runner call site.
- monitoring implements routes' `RouteMetricsReader` (M-BE-20).
- The first-login admin is the default `LoginHook` in platform-core (PC-26). access-control's Bootstrapper (AC-13) replaces it.

## Remaining open questions (not covered by the decisions)

| Folder | Question |
| --- | --- |
| platform-core | ~~Are API tokens (P2) user-owned or team-owned service accounts?~~ Resolved in P2: user-owned, narrowed by scopes (platform-core design §13). |
| pipeline-engine | How is the `email` mask applied when the local part has 1 char? |
| js-sandbox | What shape do headers take in JS: map vs array? |
| connections | Should "can write" auto-create a missing test topic? |
| live-preview | Is masking in the preview service under an API directive acceptable, or must it be strictly in the API? |
| access-control | Do pending approvals expire? Should approvers also get email? |
| route-runtime | Should restarted workers have an encrypted on-disk secret cache for when the control plane is down? |
| transform-studio | Are the new `/assist/*` paths and the API-served `/preview/infer-mapping` OK? |
