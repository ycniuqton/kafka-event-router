# Routes — Requirements (00000-init)

## Business intent
A route moves events from a source topic to one or more destination topics, filtered and
reshaped, without writing or deploying a service. This feature owns the route as a living
object: the routes list, the 3-zone route builder (Source / Filter / Destination / Review),
templates, branches, the deploy lifecycle seen from the API, versions, diff, rollback and
YAML import/export. A route must read like a sentence and nothing goes live unvalidated.
Sources: UI spec §6, §7, §11.1, §13; SysDesign §4-§6, §9.4, §14.

## Functional requirements
Phase tags follow UI spec §13 / SysDesign §14 milestones.

### Routes list (UI §6)
- FR-01 [MVP] Paginated list: status badge, name, source, filter summary, transform indicator, destination(s), throughput+sparkline, match rate, lag, owner, last changed.
- FR-02 [MVP] Search by name; filter by status, connection, tag. Owner/team filter is [P2]. Every user can read every team's routes; writes need a role on the owning team.
- FR-03 [MVP] Row actions: Pause/Resume, Edit, Duplicate, View metrics, Archive.
- FR-04 [MVP] Bulk pause, resume, add tag. Bulk export as YAML is [P2].
- FR-05 [P2] Table / card view toggle (remembered per viewer).
- FR-06 [MVP] "+ New route" chooser: Filter & forward, Publish to partner, Merge topics, Blank. Templates only pre-fill the draft; same builder afterwards.
- FR-07 [MVP] Empty state explains routes and offers "+ New route"; shortcut `N` opens the chooser, `/` focuses search.

### Route builder (UI §7) — 3-zone editor only, no wizard
- FR-10 [MVP] Layout: left pipeline rail (cards with summary + tick/warning + unsaved dot), centre step editor, right live sample panel (owned by live-preview), top bar with inline name, status badge, Save draft (`Ctrl+S`), Test, Deploy.
- FR-11 [MVP] Source: connection with health dot, topic search with rate hints, multi-topic (merge), start from Latest/Earliest/date-time, format JSON, read-only consumer group (editable under Advanced).
- FR-12 [P2] Source: topic regex pattern, specific offsets, Avro/Protobuf format override.
- FR-13 [MVP] Filter visual builder: field picker from samples, type-aware operators, adaptive value input with seen values + frequency, AND/OR, nested groups, live match counter.
- FR-14 [MVP] Quick filter from data: sample panel "Keep only events where X = Y" adds a row.
- FR-15 [MVP] Expression mode (one-line JS boolean); visual→expression always, expression→visual when simple, else say so.
- FR-16 [P2] Code mode (full JS filter function), stored as an `expr` node (no new AST node).
- FR-17 [MVP] Why-filtered: condition rows highlight green/red for a selected sample event.
- FR-18 [MVP] Destination: connection + topic, Create topic if missing, key (keep/field/none), partitioning (same/by key/round-robin), headers (keep all/selected/drop all/add), on-error (skip/DLQ default/pause).
- FR-19 [P2] Destination output format Avro (schema subject).
- FR-20 [P2] Branches: "+ Add branch" under Filter splits into parallel filter+transform+destination paths.
- FR-21 [MVP] Review: route as a sentence, diff vs running version, validation results, dry-run result on samples, Deploy with optional change note.
- FR-22 [MVP] Transform step card opens the Transform Studio (owned by transform-studio).

### Lifecycle, validation, deploy (SysDesign §5, §9.4)
- FR-30 [MVP] Create route, save draft (PATCH) with optimistic concurrency (`If-Match` on `row_version`). Save draft always saves and lists issues; it never blocks.
- FR-31 [MVP] Validate draft: errors and warnings with JSON Pointer paths; problem `path` highlights the field. External-data rules (connections, topics, field types, test cases, library refs) live here.
- FR-32 [MVP] Deploy: validate (errors block) → immutable version (v1, v2 …) + deployment → active version switched in one transaction → workers notified.
- FR-33 [P2] Deploy requires approval for production destinations when the team requires it ("Request approval"); strict 4-eyes (the requester never approves their own deploy); a rejected request still consumes its version number.
- FR-34 [P2] Deploy blocked when saved test cases fail.
- FR-35 [P2] Rollout result handling: active, or failed with automatic rollback within 60 s; auto-rollback creates a new version (origin `auto_rollback`) like a manual rollback.
- FR-36 [MVP] Pause / Resume change desired state (paused routes stop their runners); Archive stops and hides the route (no hard delete). [P2] Unarchive.
- FR-37 [MVP] Derived status: Draft, Running, Paused, Error, Archived, always shown as a badge.
- FR-38 [P3] Exactly-once delivery option in route settings (same-connection only).
- FR-39 [P3] Replay: `POST /routes/{id}/replay` is owned here; the offset reset is route-runtime's and the confirm dialog (route-detail header) is monitoring's.

### History (UI §11.1)
- FR-40 [MVP] History tab: versions with author, time, change note, origin.
- FR-41 [MVP] Readable side-by-side diff of two versions (source, filter rows, steps, destination).
- FR-42 [MVP] Roll back to version N redeploys it as a new version; history never lost; failing test cases are warnings on rollback.
- FR-43 [P2] Export route as YAML / JSON with connection names (not ids); import from YAML resolves names within the team (creates or updates a draft, never deploys).
- FR-44 [MVP] Route detail Settings tab: name, tags, replicas, error policy, limits; hosts monitoring's alert form (P2).
- FR-45 [MVP] Route detail page shell: header (name, status, Pause/Resume, Edit, action slot) + tab registry used by monitoring, dead-letters and this folder.

## Non-functional / constraints
- Go 1.27 + chi + slog; Postgres via internal/store; React 19 + TS + CSS Modules on the FE.
- Config document is the single SysDesign §5 JSON schema; YAML is the same document.
- Versions are immutable; rollback never edits history. Every mutation is audited.
- Coding rules: base interface + Manager per extensible domain, config-selected implementations, enums/constants, class-based handlers, functions ≤ 50 lines.
- List p95 < 300 ms for 1,000 routes; validate p95 < 1 s excluding test cases.
- Desktop-first (≥1280 px); on mobile only status + pause/resume must work.

## Out of scope
- Wizard builder; route templates stored server-side.
- Hard delete of routes (Archive only).
- Running routes, offsets, rollout mechanics (route-runtime); preview engine (live-preview).

## Dependencies
- pipeline-engine: route config schema + generated Go/TS types, doc-only validators, engine.Compile, filter AST, YAML/JSON document format.
- platform-core: auth context, pagination, If-Match/ETag, RFC 7807, audit middleware, migrations runner.
- connections: connection lookup, env tag, topic list, create topic. access-control: `CanUseConnection` (P2).
- live-preview: sample panel, field schema, preview run (dry run, match counter, why-filtered).
- transform-studio: transform step editor, library ref lookup. access-control: RBAC, approvals.
- route-runtime: NOTIFY consumer, rollout reporting, offset reset. monitoring: per-route summary metrics, replay dialog.
