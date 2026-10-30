# Dead letters — Design (00001-workflow)

Incremental on 00000-init. **Unchanged:** sections 1–12 of 00000-init (record contract, reader, state store, actions,
jobs, masking, export, studio, demo, exactly-once, perf gate). Only additions and changed contracts are listed here.

## 1. Overview

```
 Browser  pages/dead-letters (Causes tab · list · detail editor · replay dialog · approvals)
   │  REST /api/v1/routes/{id}/dlq/groups · /dlq/groups · …/{dlqId}/replay · …/dlq/dry-run · /dlq/jobs/{id}/approve|reject
   ▼
 internal/dlq/httpapi ── dlq.Manager
                          ├─ CauseGrouper     (scan → Fingerprinter → GroupSet; GroupCache 15 s)
                          ├─ record.ErrorClass (pure: ErrorType → data | router | transient)
                          ├─ EditPlanner       (EditValidator → MaskMerger → StructuralDiff)
                          ├─ DryRunner         (implemented by the configured RetryExecutor; preview reuse)
                          └─ ReplayGate        (approvals.Manager.Gate over the active version's destinations)
 internal/dlq/jobs  ── rate-limited pages, pending_approval → approve/reject, edited single-record jobs
 internal/alerts    ── kind router_caused_dead_letters + DefaultRuleProvisioner (workspace default on)
 cmd/worker (route-runtime) ── failures.DeadLetterTypes (finer error types) + retrytopic.Scheduler/Consumer
```

## 2. Error-type and classification contract (shared, `internal/dlq/record`)

`record.ErrorType` gains writer types; `record.ErrorClass` is a pure function of the type (no header — old records
classify the same way, the alert can be PromQL on `router_events_dlq_total{error_type}`):

| ErrorType | Class | Written when (worker) |
| --- | --- | --- |
| `step_error` | data | user script/step threw, ErrLimit (script exceeded its own limits) |
| `decode_error` | data | source bytes do not decode with the source format |
| `output_invalid` | data | output fails destination schema/format validation |
| `timeout` | transient | step timeout after in-memory retries |
| `lookup_unavailable` | transient | `engine.DependencyError{Kind: lookup}` in the chain (F9 lookups) |
| `dependency_unavailable` | transient | any other `engine.DependencyError` (schema registry outage, …) |
| `produce_retriable` | transient | retriable produce error parked by a retry policy / API replay |
| `internal_error` | router | panic in the record path, untyped error |
| `sandbox_crash` | router | `sandbox.Error` kind `internal` or `host` |
| `encode_error` | router | `pipeline.Encode` failure that is not a validation error |
| `produce_error` | router | permanent produce failure recorded on a record |
| `config_error` | router | compile/config error at run time (`engine.CompileError`, sandbox kind `compile`) |
| `unknown` | router | reader-only: unreadable DLQ record |

```go
type ErrorClass string // data | router | transient
var ErrorClasses = map[ErrorType]ErrorClass{…}
func (t ErrorType) Class() ErrorClass           // unmapped → router (fail loud: unknown = our bug)
func TypesOf(c ErrorClass) []ErrorType          // filters, PromQL regex of the alert
```

New header `router.retry_attempts` (`record.HeaderRetryAttempts`, `Failure.Attempts`): attempts used by an
auto-retry policy before the record was parked (absent = 0).

### 2.1 Transient-error contract for other features (F9 lookups)

`pkg/engine/dependency.go` (new file, no change to `errors.go`):

```go
type DependencyKind string            // lookup | schema_registry | http
type DependencyError struct { Kind DependencyKind; Name string; Cause error }
func (e *DependencyError) Transient() bool { return true }   // also matched by failures.TransientCauses
func AsDependencyError(err error) (*DependencyError, bool)
```

A step whose external dependency cannot answer (unreachable, timeout, 5xx, pool exhausted) returns a
`ProcessError` whose cause chain holds `&engine.DependencyError{Kind: engine.DependencyLookup, Name: <sourceId>, Cause: err}`.
Lookup steps (pipeline-engine 00002-lookups §4) use engine type `engine.ErrLookupUnavailable` (`lookup_unavailable`)
for it; other steps may keep `step`. A lookup **miss** (`lookup_miss`, `onMiss=fail`) or a bad key is **not** a
DependencyError: category `step` → DLQ type `step_error` → class data.

```
[event] lookup step onError=fail after retries, transient cause
└─ ProcessError{Type: lookup_unavailable, Cause: DependencyError{Kind: lookup}}
   └─ DefaultClassifier → dependency_transient → apply_policy
      ├─ errorPolicy.retry enabled, attempts left → retry tier topic (OutcomeRetryScheduled)
      └─ else DLQ writer: DependencyRule → router.error_type=lookup_unavailable → class transient
[API] DLQ dry-run / retry of that record (in-process gateway since integration)
└─ DependencyKindOf → failure kind lookup_unavailable
```

Integration note: F9's interim `lookup_transient` type and `CategoryLookupTransient` were folded into this
contract; there is exactly one transient lookup type.

## 3. Cause grouping

### 3.1 Fingerprint

```go
type MessageNormalizer struct{}              // ordered NormalizationRule list (regexp → placeholder)
type Fingerprinter struct{ Normalizer MessageNormalizer }
func (Fingerprinter) Of(routeID string, f record.Failure) Fingerprint   // {ID, Pattern}
```

Rules, in order: ISO timestamps → `<ts>`; UUIDs → `<uuid>`; prefixed ids (`ord_123`, `usr-9f2a`) → `<id>`;
hex runs ≥ 8 with a digit → `<hex>`; `partition 3`, `offset 884162`, `p@o` → `<n>`; any remaining number
(int/float, incl. `:7:22` positions) → `<n>`; collapse whitespace; truncate to 240 runes.
`ID = hex(sha256(routeID \x1f step \x1f errorType \x1f pattern))[:16]`. Stack and payload are never used.

### 3.2 Grouping flow

```
HTTP GET /api/v1/routes/{id}/dlq/groups?since&until&errorType&class
└─ GroupsHandler → rbac(dlq.read) → Manager.Groups(routeID, GroupQuery)
   └─ GroupCache hit (route+query, 15 s) → cached GroupPage
   └─ Locator.Locate → Reader.Scan pages (unresolved, Query filters) until MaxGroupScan=10 000 or budget
      └─ each record → Fingerprinter.Of → GroupSet.Add (count, first/last, versions, newest sample, TrendBuckets)
      └─ scan budget hit → truncated=true
   └─ GroupSet.Page → sort count desc, lastSeen desc → top MaxGroups=200 → byClass totals
   └─ 200 { items, scanned, truncated, approximate, byClass }
---
HTTP GET /api/v1/dlq/groups?routeId=…&class=…
└─ GlobalGroupsHandler → ReadableRoutes.Filter → Manager.GlobalGroups
   └─ per route (parallel ≤ 8, MaxGroupScanPerRoute=2 000) → merge pages → same sort/top
   └─ one route fails → warnings[{routeId, reason}]
```

Trend: 24 buckets over `[since, until)` (default the last 24 h → hourly); records outside count but no bucket.

### 3.3 API

```json
GET /routes/{id}/dlq/groups →
{ "items": [{ "fingerprint": "3f9ac1d2e4b5a6f7", "routeId": "r3", "routeName": "Orders", "step": "js1",
    "stepLabel": "Step 1 · JavaScript", "errorType": "step_error", "errorClass": "data",
    "pattern": "Cannot read properties of undefined (reading 'reduce')", "count": 412,
    "firstSeenAt": "…", "lastSeenAt": "…", "versions": [4, 5],
    "sample": { "id": "…", "message": "…", "failedAt": "…" },
    "trend": { "since": "…", "bucketSeconds": 3600, "counts": [0, 3, …] } }],
  "scanned": 5000, "truncated": false, "approximate": true,
  "byClass": { "data": 400, "router": 12, "transient": 0 }, "warnings": [] }
```

List/match additions: query `fingerprint=` and `class=data,router` on `GET /routes/{id}/dlq`, `GET /dlq`, export and
the `match` body (`"fingerprint"`, `"errorClass": []`), stored in job `match`. Rows and details carry `errorClass`,
`fingerprint`, `attempts`; summaries carry `byErrorClass`.

## 4. Edit-then-replay

### 4.1 Types

```go
type RecordEdit struct { Key *EncodedValue; Value *EncodedValue; Headers *[]EncodedHeader } // nil = keep original
type EditPlan struct { Record record.Record; Diff []DiffEntry; Masked bool }
type DiffEntry struct { Path string; Op DiffOp }    // op: add | remove | replace; paths /key, /value/items/0, /headers/traceId
type EditValidator struct{}  // encoding vs source format (json source ⇒ json data), MaxEditBytes=1 MiB, avro/protobuf → 422
type MaskMerger struct{ Masker ValueMasker } // leaf == Mask(original leaf) ⇒ original leaf (masked editors)
type StructuralDiff struct{} // JSON tree diff, paths only (no values in audit)
type EditPlanner struct{…}   // validate → (masked ? merge) → bytes → diff
```

### 4.2 Replay a single record (edited or not)

```
HTTP POST /api/v1/routes/{id}/dlq/{dlqId}/replay { edit?, dryRun, note }
└─ ReplayHandler → rbac(dlq.act) → 403 for viewers
   └─ Manager.Replay(viewer, routeID, id, ReplayRequest)
      └─ fetchOne(ref) → not found/expired → 404
      └─ ViewPolicy decides masked for this viewer on the DLQ connection
      └─ EditPlanner.Plan(original, edit, masked)
         └─ bad encoding / not JSON for json source → 400 path=/edit/value
         └─ avro/protobuf source → 422 edit_unsupported_format ; > 1 MiB → 422 path=/edit
         └─ masked → MaskMerger restores untouched masked leaves from the original
      └─ dryRun → DryRunner.DryRun(activeVersion, [planned]) → outputs masked like the source view
         └─ 200 { dryRun: true, result{outcome would_send|filtered|failed, outputs[]}, diff, masked }
      └─ ReplayGate.Check(route, activeVersion) → require_approval
         └─ jobs.Service.CreatePending(ids=[id], edit, note) → dlq_jobs(status=pending_approval, edit)
         └─ 202 { status: "pending_approval", job }
      └─ allowed → ActionManager retry path with [planned] (same Ref: lock, mark retried{edited})
         └─ audit dlq.edited_replayed {id, diff paths, note, outcome}   (edited only; unedited → dlq.retried)
         └─ 200 { dryRun: false, status: "done", report, diff }
```

### 4.3 Dry-run of a selection or group

```
HTTP POST /api/v1/routes/{id}/dlq/dry-run { ids | match }
└─ DryRunHandler → rbac(dlq.act) → Selector.Resolve sample ≤ DryRunSample=20 (newest)
   └─ DryRunner.DryRun(activeVersion, records) (no produce, no state)
   └─ 200 { activeVersionNo, sampled, results[{id, outcome, errorType, message, outputs[]}], counts }
```

`DryRunner` is an optional interface of the configured `RetryExecutor` (`InProcessExecutor`: compile via
PipelineCache, decode → Process → BranchScope → Encode, no produce; `PreviewExecutor`: the preview run endpoint, i.e.
the same preview the Studio uses). Outputs are rendered with `ValueEncoder` and masked with the record's view decision.

### 4.4 Bulk replay jobs (changed)

`POST /routes/{id}/dlq/jobs` body adds `ids` (≤ MaxJobIDs=10 000) as an alternative to `match`, `ratePerSecond`
(1–1000, default 50), `note`; `match` gains `fingerprint`, `errorClass`. Job DTO adds `ratePerSecond`, `selection`
(`ids`|`match`), `edited`, `note`, `approval {required, decidedBy, decidedAt, decisionNote}`.

```
[async] jobs.Runner.Execute(job)
└─ per page: RateLimiter (token bucket, job.ratePerSecond) → page size = min(MaxSyncAction, rate·2)
   └─ ids selection → Manager.ActIDs(page of ids) ; match → Manager.ActPage (unchanged)
   └─ edited job → Manager.ReplayEdited(job.edit) once → Finish → edit cleared
   └─ Progress(processed,total,counts) → FE polls GET /dlq/jobs/{id} every 2 s (progress bar)
```

### 4.5 Strict 4-eyes for replays

```
POST …/replay | POST …/dlq/jobs (action=retry) | POST …/dlq/retry
└─ ReplayGate.Check(caller, route, active version destinations)
   └─ approvals.Manager.Gate(DeployTarget) → deploy_now → run (job queued)
   └─ require_approval → job status pending_approval (sync retry: 409 replay_approval_required)
   └─ deny → 403
---
HTTP POST /api/v1/dlq/jobs/{jobId}/approve { note }  |  …/reject { note }
└─ ApproveJobHandler → rbac(deployment.approve on route team)
   └─ caller == requester → 403 self_approval (admins included)
   └─ status ≠ pending_approval → 409 conflict
   └─ approve → status queued (runner claims it) ; audit dlq.replay_approved
   └─ reject → status rejected, edit cleared ; audit dlq.replay_rejected
```

`ReplayGate` is a port (`AlwaysAllowGate` when approvals are not wired); the adapter lives in `cmd/api/wire_dlq.go`.

## 5. Alerts: router-caused dead letters

- Kind `router_caused_dead_letters`, threshold bound 0…1e6 (default 0), window default 15 min.
- PromQL (`query.QueryAlertRouterDLQ`): `sum(increase(router_events_dlq_total{route_id="…",error_type=~"<router types>"}[w]))`.
- `RouterCausedEvaluator.Breached` = present ∧ value > threshold.
- Workspace setting `router_caused_alert_default` (bool, default true) in the access-control setting catalog.

```
[cron] AlertEvaluatorJob.Tick (leader)
└─ DefaultRuleProvisioner.Ensure (setting on)
   └─ INSERT alert_rules for routes without alert_rule_provisions(route, kind) → INSERT provision rows
      (a deleted rule keeps its provision row → never re-created; setting off → nothing provisioned)
└─ ListEnabled → evaluate (unchanged)
```

## 6. Auto-retry policy (route-runtime)

### 6.1 Config (`errorPolicy.retry`, schema + Go + TS)

```json
"errorPolicy": { "onError": "dlq", "retry": { "enabled": true, "maxAttempts": 5, "baseDelayMs": 1000, "maxDelayMs": 300000 } }
```
Bounds: maxAttempts 1–10 (default 5), baseDelayMs 100–3 600 000 (default 1000), maxDelayMs ≥ base, ≤ 86 400 000
(default 300 000). `delay(n) = min(base · 2^(n−1), max)`, n = attempt number 1..maxAttempts.

### 6.2 Retry topics and flow

Tier topic per attempt: `router.retry.<route-slug>.<n>` on the DLQ connection, partitions = source partitions,
retention 7 days, created on runner start when retry is enabled. Each tier has one fixed delay, so offsets are in due order
and a tier partition only waits for its head record.

```
recordTicket.run → failure classified
└─ category ∈ {step_timeout (after in-memory retries), dependency_transient, destination_transient from processing}
   ∧ errorPolicy.retry.enabled
   └─ retrytopic.Scheduler.Schedule(raw, attempt=1) → produce to tier 1 with headers
      router.retry.attempt, router.retry.due_at (unix ms), router.retry.error_type, router.retry.error_message,
      router.source_topic/partition/offset + original headers
      └─ ack → source record finished (outcome retry_scheduled) → partition never blocked
   └─ else → unchanged path (policy / blocking backoff)
---
[async] retrytopic.Consumer (per runner, group "<route group>.retry", all tiers)
└─ tier partition head not due → pause that tier partition until due_at
└─ due → current versionState.processor.Process(original raw)
   └─ ok → deliver via the runner's delivery strategy → commit tier offset → recovered++
   └─ transient again ∧ attempt < max → Schedule(attempt+1) → commit
   └─ transient again ∧ attempt = max → onError handler (dlq: parked, type = transient type, router.retry_attempts = max) → exhausted++
   └─ non-transient failure → onError handler (normal dead letter with attempts)
```

Metrics: `router_events_retry_scheduled_total{route_id,version,worker,error_type,attempt}`,
`router_events_retry_recovered_total{…}`, `router_events_retry_exhausted_total{…,error_type}`,
`router_retry_pending{route_id}` (gauge: tier lag). Retries are at-least-once (outside exactly_once transactions).
Produce/destination outages keep the blocking partition backoff (diverting would reorder and flood every record).

### 6.3 Worker error types

`failures.DeadLetterTypes.Of(Failure)` replaces the category-only mapping: panic (`failures.PanicError`) →
internal_error; sandbox internal/host → sandbox_crash; compile → config_error; DependencyError → lookup_unavailable /
dependency_unavailable; non-validation encode → encode_error; otherwise category mapping (unchanged).

## 7. Data model

Migration `3100_dlq_workflow` (P4 range, dead-letters block 3100..3109; renumbered from 2100 at integration):

```sql
ALTER TYPE dlq_job_status ADD VALUE 'pending_approval'; ALTER TYPE dlq_job_status ADD VALUE 'rejected';
ALTER TABLE dlq_jobs ADD COLUMN rate_per_second integer NOT NULL DEFAULT 50, ADD COLUMN edit jsonb,
  ADD COLUMN note text NOT NULL DEFAULT '', ADD COLUMN decided_by uuid REFERENCES users ON DELETE SET NULL,
  ADD COLUMN decided_at timestamptz, ADD COLUMN decision_note text NOT NULL DEFAULT '';
ALTER TYPE alert_rule_kind ADD VALUE 'router_caused_dead_letters';
CREATE TABLE alert_rule_provisions (route_id uuid REFERENCES routes ON DELETE CASCADE, kind alert_rule_kind, provisioned_at timestamptz, PRIMARY KEY (route_id, kind));
```
Job `selection` jsonb holds `{ids}` or the match (fingerprint, errorClass added).

## 8. Audit (feature-owned, registered at wiring)

`dlq.edited_replayed` {id, diff[{path, op}], note, outcome, masked} · `dlq.replay_requested` {jobId, count|ids, note} ·
`dlq.replay_approved` / `dlq.replay_rejected` {jobId, note}. Values never stored.

## 9. Frontend

| Area | Files |
| --- | --- |
| Domain | `domain/deadLetter.ts`: `DeadLetterErrorClass` + `DeadLetterErrorClassMeta` (Bad data/warn, Router bug/err, Transient/info), `DeadLetterGroup`, `DeadLetterEdit`, `DryRunResult`, job additions; `domain/deadLetterGroups.ts` grouping model |
| Contract | `DeadLettersService`: `groups(filter)`, `replay(dl, edit, {dryRun, note})`, `dryRun(routeId, selection)`, `startJob(…, {ids?, ratePerSecond, note})`, `approveJob`, `rejectJob`, `pendingApprovals()`; mock + HTTP |
| Screens | `pages/dead-letters/components/CauseGroupList.tsx` (Causes view: badge, count, trend sparkline, first/last, sample, actions Show events / Dry-run / Replay group) · `ErrorClassBadge.tsx` · `RecordEditor.tsx` (JSON editor, validation, masked notice) · `ReplayDialog.tsx` (dry-run preview, diff, note, rate, approval state) · `ReplayJobProgress` (in `DeadLetterJobPanel`) · `ReplayApprovalsPanel.tsx` |
| Builder | route settings → error handling: `RetryPolicyFields` (enabled, attempts, base, max, computed schedule) |
| Alerts | `domain/alertRule.ts` kind `router_caused_dead_letters`; Settings → workspace toggle "Alert on router-caused dead letters" |

## 10. API contract summary (additive, `/api/v1`)

| Method + path | Perm | Body → response |
| --- | --- | --- |
| `GET /routes/{id}/dlq/groups?since&until&errorType&class` | dlq.read | → `GroupPage` (§3.3) |
| `GET /dlq/groups?routeId&since&until&errorType&class` | dlq.read per route | → `GroupPage` + `warnings` |
| `GET /routes/{id}/dlq?…&fingerprint&class` (also `/dlq`, export) | unchanged | rows + `errorClass`, `fingerprint`, `attempts` |
| `GET …/dlq/summary` (route, global) | unchanged | + `byErrorClass {data, router, transient}` |
| `POST /routes/{id}/dlq/{dlqId}/replay` | dlq.act | `{edit?, dryRun, note}` → 200 dry `{dryRun:true, activeVersionNo, masked, diff, result}` · 200 `{dryRun:false, status:"done", diff, report}` · 202 `{dryRun:false, status:"pending_approval", diff, job}` |
| `POST /routes/{id}/dlq/dry-run` | dlq.act | `{ids}`\|`{match}` → `{activeVersionNo, sampled, results[], counts{would_send, filtered, failed}}` |
| `POST /routes/{id}/dlq/jobs` | dlq.act | `{action, ids?\|match?, ratePerSecond?, note?}` → 202 job (status queued or pending_approval) |
| `GET /dlq/jobs?status=pending_approval` | dlq.read per route | → `{items: Job[]}` |
| `POST /dlq/jobs/{jobId}/approve` · `/reject` | deployment.approve (route team), not requester | `{note}` → job; 403 `self_approval`; 409 not pending |
| `POST /routes/{id}/dlq/retry` | unchanged | 409 `replay_approval_required` when the 4-eyes gate requires approval |

`edit` = `{ key?: EncodedValue|null, value?: EncodedValue, headers?: EncodedHeader[] }` (same shapes as detail §4.3 of
00000-init; omitted member = keep original). `result`/`results[]` = `{ id, outcome: would_send|filtered|failed,
errorType?, message?, outputs: [{ branch, connectionId, topic, key: EncodedValue|null, value: EncodedValue, headers }] }`.
Job DTO additions: `ratePerSecond`, `note`, `edited`, `idsCount`, `approval {required, decidedBy?, decidedAt?, decisionNote?}`;
statuses add `pending_approval`, `rejected`. Workspace settings add `routerCausedAlertDefault` (bool).

## 11. As built (deviations and additions to §1–10)

- Audit: dry runs (single and selection) are audited as `dlq.replay_previewed` (category access, counts/ids only);
  pending replays write `dlq.replay_requested` (with the diff paths for an edited one).
- Jobs: discard jobs are never throttled (nothing is produced); only retry jobs honour `ratePerSecond`
  (`jobs.PacerOf`, page = rate × 2 s, ≤ 500). Job DTO adds `requestedByMe` (set per caller) so the UI hides
  Approve/Reject on one's own replays; `ids` are stored in `selection.ids` and never echoed (`idsCount`).
- An approved edited job (`edit` + one id) runs through `Manager.ReplayApproved`; the stored edit is the planned,
  unmasked record (`PlannedEdit`) and is cleared (`edit = NULL`) on finish, reject or cancel.
- Masking of dry-run outputs follows the viewer's decision on the DLQ connections of the route (any masked → masked).
- Runtime: an `output_invalid` with a transient cause keeps the blocking in-place backoff (unchanged); only
  `pipeline.Encode`'s unknown-branch failure maps to `encode_error` (validation and codec failures are reported the
  same way, so they stay `output_invalid`). Enabling/disabling retry or changing `maxAttempts` forces a full runner
  restart; delay changes are a pipeline swap. Tier records carry `router.retry.source_ts` too.
- UI: the retry policy fields live in route detail → Settings → Error policy (the surface that edits `errorPolicy`).
