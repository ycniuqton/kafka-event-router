# access-control — Requirements (00000-init)

Sources: UI spec §11.2 (roles, teams, approvals), §11.3 (audit log), Settings screen, §13 phases;
SysDesign §4 (`teams`, `users`, `role_bindings`, `deployments`, `audit_log`), §6 (Approvals, Admin), §9.4 step 3, §10 (masking), §11.
Code: `internal/rbac`, `internal/approvals`, `internal/audit`, `web/src/pages/audit`, `web/src/pages/settings`, `web/src/pages/approvals`, `web/src/access`.

## Business intent
The platform holds credentials to other teams' clusters and moves their data, so every action must be
allowed by a clear role, production changes must be reviewable before they go live, sensitive event
data must stay masked for people who only need to observe, and every action must be traceable
afterwards (who, what, when, from where). Admins manage people, teams and workspace settings from one Settings screen.

## Functional requirements
### Roles & policy engine
- FR-01 [MVP] Four team-scoped roles: Viewer, Editor, Approver, Admin, plus a workspace-admin flag; role → permission matrix is data, not code branches.
- FR-02 [MVP] Every `/api/v1` call is authorized through `rbac.Manager` (implements platform-core's `auth.Authorizer`); deny → 403 problem naming the missing permission.
- FR-03 [MVP] Single default team; only Admin and Editor are assignable in the UI (UI §13 MVP).
- FR-04 [MVP] Effective permissions per team are embedded in `GET /auth/me` so the UI hides/disables actions; server stays the authority.
- FR-05 [MVP] Bootstrap: the first user to log in (and dev login / `ROUTER_BOOTSTRAP_ADMIN_EMAILS`) becomes workspace admin; every other user gets roles manually from an admin (OIDC group mapping later, platform-core P3); the last workspace admin can never be removed or disabled.
- FR-06 [MVP] Binding/role changes take effect on all API instances within 2 s (NOTIFY invalidation of the subject cache).
- FR-07 [P2] Full roles: Viewer and Approver assignable; Approver inherits Editor rights in its teams.
- FR-08 [P2] Multiple teams; routes, connections, transforms belong to one team; users may hold one role per team.
- FR-09 [P2] Connection grants: table `team_connection_grants`, `/teams/{id}/grants` API and `CanUseConnection` are owned here; a route may use only connections owned by or granted to its team (rule runs in routes validation). Any user may read routes/connections of every team; writes are team-scoped.
- FR-10 [P3] Policy engine implementation is config-selected (`ROUTER_RBAC_ENGINE`), so an external engine (e.g. OPA) can replace the built-in role-binding engine.
### Users, teams, settings (Admin)
- FR-11 [MVP] `GET /users`, `PATCH /users/{id}` (display name, workspace admin, disable); users are provisioned by OIDC login (platform-core).
- FR-12 [MVP] `GET/PUT/DELETE /role-bindings` (user, team, role); `If-Match` on changes; audited with before/after.
- FR-13 [P2] `GET/POST /teams`, `GET/PATCH/DELETE /teams/{id}` (name, require-production-approval); delete blocked while the team owns objects.
- FR-14 [P2] Pre-invite a user by email with a role before first login (pending user row, matched on login by email).
- FR-15 [MVP] Settings screen: Appearance (theme, phase tags — per browser), Members & roles (list, change role; admin only edits).
- FR-16 [P2] Settings screen: Teams section (create/rename, members, approval policy, connection grants).
- FR-17 [P3] Settings screen: Workspace section (AI assist on/off, default off; audit export target status).
### Approvals (UI §11.2, SysDesign §9.4 step 3)
- FR-20 [P2] Deploy gate: a deploy whose destination connection is `prod` and whose team requires approval becomes "Request approval" for everyone (strict 4-eyes, Approvers/Admins included); Editors deploying to `prod` always need approval.
- FR-21 [P2] Request creates the immutable version + deployment `pending_approval` (a rejected request still consumes its version number); a newer request for the same route cancels the older pending one.
- FR-22 [P2] Approvers of the route's team (or workspace admins) get a bell notification linking to a review page with the diff vs the active version.
- FR-23 [P2] Approve → deployment activated through the routes deploy pipeline (re-validated); Reject requires a note; anyone involved can comment; requester can withdraw.
- FR-24 [P2] Requester can never approve their own request (strict 4-eyes, no admin bypass); decisions are race-safe (exactly one decision wins).
- FR-25 [P2] `GET /approvals` (filters: status, team, route, mine), `GET /approvals/{id}`, `POST /approvals/{id}/approve|reject|withdraw|comments`.
### Sample masking & sensitive access
- FR-30 [P2] `rbac.Manager.SampleVisibility` decides Raw vs Masked per user and connection: Viewers always masked; every other role masked only on connections marked `sensitive` (column owned by connections); masks keep types.
- FR-31 [P2] Every sample load, live-tail open and DLQ payload view records an audit entry; raw access to a sensitive connection is flagged `sensitive`.
### Audit log (UI §11.3)
- FR-40 [MVP] Audit action/object catalog (`audit.Action`, `audit.ObjectType`) used by every feature's writes; sensitivity derived from the action.
- FR-41 [MVP] Internal `audit.Manager.Query` (filters + cursor) used by monitoring's activity feed.
- FR-42 [P3] `GET /audit-log` (workspace admins only) filterable by actor, action, object type/id, team, time range, sensitive-only, text; newest first, cursor pages.
- FR-43 [P3] Audit log screen: read-only table (time, actor, action, object, IP), filters, sensitive rows highlighted amber, infinite scroll.
- FR-44 [P3] Export the filtered audit log as JSONL/CSV (streamed) and continuous SIEM export via a config-selected sink (webhook, syslog).

## Non-functional / constraints
- Authorization check p99 < 2 ms (in-process cached subject); cache TTL ≤ 30 s plus NOTIFY invalidation.
- Fail closed: unknown permission, missing principal, or engine error → deny.
- Audit log append-only (DB trigger), written in the same transaction as the change (platform-core FR-31).
- Audit query p95 < 500 ms over 10 M rows using `(at DESC)` and filter indexes.
- Devlens coding rules: base interface + Manager per extensible domain (policy engine, audit sinks, approval policy), config-selected implementations, enums for roles/permissions/actions, class-based handlers.
- Coverage: role matrix and deploy gate 100% table-driven; approval state machine fully covered.

## Out of scope
- Authentication, sessions, API tokens, CSRF, audit write mechanics → platform-core.
- The masking algorithm itself → live-preview (samples), monitoring (tail), dead-letters (payloads).
- Diff rendering and version activation → routes. Notification transport (WS, bell) → monitoring.
- SCIM / group sync from the IdP, per-route ACLs, multi-workspace tenancy, multi-approver quorum.

## Dependencies
- platform-core: `auth.Principal`, `auth.Authorizer` port, `LoginHook`, `/auth/me`, audit recorder + migration 0003, NOTIFY publisher, Problem types.
- routes: `POST /routes/{id}/deploy` calls `approvals.Manager.Gate`; implements `approvals.Activator`; version diff component.
- connections: `env_tag`, `connections.sensitive` (column + form toggle owned there; read here), owner team.
- monitoring: `WS /events` + bell for approval notifications; activity feed reads `audit.Manager.Query`.
- live-preview, monitoring, dead-letters: call `SampleVisibility` and record sample-view audit entries.
- transform-studio: reads the workspace AI-assist setting.
