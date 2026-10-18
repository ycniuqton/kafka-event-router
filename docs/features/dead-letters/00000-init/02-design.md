# Dead letters — Design (00000-init)

Read side of the dead-letter queue (UI spec §10.3, SysDesign §8.4). The write side (runner → DLQ topic) belongs to
**route-runtime**; this folder owns the shared DLQ record contract that both sides use.

## 1. Overview

```
 Browser (web/src/pages/dead-letters, route-detail Dead tab)
   │  REST /api/v1/routes/{id}/dlq*, /api/v1/dlq*
   ▼
 cmd/api ── internal/dlq/httpapi (class handlers) ── dlq.Manager
                                                      ├─ Locator        (route → DLQ connection + topics, Postgres)
                                                      ├─ Reader         (KafkaReader: manual assign, no group)
                                                      ├─ StateStore     (compacted router.dlq.state topic)
                                                      ├─ ActionManager  (Retry / Discard / Restore handlers)
                                                      │    └─ RetryExecutor (in-process engine + producer)
                                                      ├─ ExportManager  (json / ndjson writers)
                                                      └─ StudioLinker   (→ live-preview inline sample session)
 cmd/worker (route-runtime) ── internal/dlq/record.Codec.Encode ──► DLQ topic (source connection by default)
```

Nothing is persisted in Postgres for MVP. P2 adds one table for bulk jobs (section 6).

## 2. Go packages and key types

All under `internal/dlq/`. Every extensible domain has a base interface, a registry keyed by a config value, and a
Manager that business logic talks to (rules 12–15). Constants live in typed enums (rules 17–18).

### 2.1 `internal/dlq/record` — shared record contract (imported by worker and API)

```go
type HeaderKey string            // router.route_id, router.version, router.branch, router.step,
                                 // router.error_type, router.error_message, router.stack,
                                 // router.source_topic, router.source_partition, router.source_offset, router.failed_at
type ErrorType string            // decode_error | step_error | timeout | output_invalid | unknown (unreadable record)
type Record struct {
    Ref        Ref               // DLQ topic/partition/offset (identity in the DLQ)
    Key, Value []byte            // original bytes, unchanged
    Headers    []engine.Header   // original headers (router.* stripped)
    Failure    Failure           // RouteID, VersionNo, Branch, Step, ErrorType, Message, Stack, FailedAt
    Source     SourcePosition    // Topic, Partition, Offset
}
type Codec struct{}              // Encode(Record) kgo-agnostic headers; Decode(headers,key,value) (Record, error)
type Limits int                  // MaxMessageBytes=2048, MaxStackBytes=8192 (truncate on Encode)
```

`router.branch` and `router.stack` are additions to SysDesign §8.4's header list (needed for branch-aware retry and
the JS stack trace in the detail panel). route-runtime must write them via `Codec.Encode`.

### 2.2 `internal/dlq` — domain

| Type | Kind | Responsibility |
| --- | --- | --- |
| `Ref` + `RefCodec` | value + class | `{Topic, Partition, Offset}` ↔ opaque id `base64url("topic\|p\|o")` |
| `Query` | value | ErrorTypes, Step, VersionNo, Since, Until, Text, IncludeResolved, Limit, Cursor |
| `Cursor` + `CursorCodec` | value + class | per (topic, partition) exclusive upper offset; base64url JSON; versioned |
| `Selection` | value | `IDs []string` **or** `Match *Query` (all matching); never both |
| `Location` | value | DLQ `ConnectionID`, `Topics []string` (distinct `errorPolicy.dlqTopic` over all versions) |
| `Locator` | interface | `Locate(ctx, routeID) (Location, error)`; impl `StoreLocator` (route_versions JSONB) |
| `Reader` | interface | `Scan(ctx, Location, Query) (ScanPage, error)`, `Fetch(ctx, Location, []Ref) ([]record.Record, error)`, `Summary(ctx, Location) (Watermarks, error)` |
| `KafkaReader` | impl | franz-go via `pkg/kafka` factory, manual assignment, backwards windows, `ListOffsets` by time |
| `StateStore` | interface | `Lookup(ctx, conn, []Ref) (map[Ref]Resolution)`, `Mark(ctx, conn, []Resolution)`, `Clear(ctx, conn, []Ref)`, `CountResolved(ctx, conn, routeID)` |
| `CompactedTopicStore` | impl | materialized index of `router.dlq.state` per connection (in-memory map + tail consumer) |
| `MemoryStateStore` | impl | tests and `ROUTER_DLQ_STATE_BACKEND=memory` local dev |
| `StateRegistry` | registry | `StateBackend` enum → factory; selected by config |
| `Resolution` | value | Ref, RouteID, `State` (retried \| filtered \| discarded), `By`, `At`, `DestRef?`, `JobID?` |
| `ActionHandler` | interface | `Kind() ActionKind`; `Apply(ctx, ActionContext, []record.Record) ActionReport` |
| `RetryAction`, `DiscardAction`, `RestoreAction` (P2) | impls | one business step each |
| `ActionManager` | manager | resolves Selection → records (cap), takes locks, dispatches by `ActionKind`, marks state, audits |
| `RetryFailureKind` | enum | step_error, timeout, output_invalid, compile, destination (per-item `failed` reason) |
| `RetryExecutor` | interface | `Execute(ctx, RetryTarget, []record.Record) []RetryOutcome` |
| `InProcessExecutor` | impl | compile active version (via `PipelineCache`), run `Pipeline.Process`, produce to destination |
| `PreviewExecutor` (P2, optional) | impl | delegates execution to `cmd/preview` internal endpoint (isolation) |
| `ExecutorRegistry` | registry | `ExecutorKind` enum (`in_process`, `preview`) → factory; config `ROUTER_DLQ_RETRY_EXECUTOR` |
| `PipelineCache` | class | LRU of compiled `engine.Pipeline` by `route_versions.id` (size 64) |
| `RetryLock` | interface | `Acquire(ctx, []Ref) (Release, []Ref /*busy*/)`; impl `RedisRetryLock` (SET NX PX 60s) |
| `Exporter` | interface | `Format() ExportFormat; ContentType() string; Write(io.Writer, iter) error` |
| `JSONArrayExporter`, `NDJSONExporter` (P2) | impls | streaming writers |
| `ExportManager` | manager | format registry, caps (`Limits.MaxExport`), audit |
| `StudioLinker` | interface | `Link(ctx, routeID, record) (StudioLink, error)`; impl calls live-preview inline-session API |
| `ViewPolicy` | interface | wraps access-control masker: `Present(ctx, principal, conn, record) (View, bool masked)` |
| `Manager` | manager | façade used by HTTP: `List`, `ListGlobal`, `Summary`, `Get`, `Act`, `Export`, `OpenInStudio` |
| `Limits` | enum | DefaultPage=50, MaxPage=200, MaxScanBytes=10 MiB, MaxScanTime=10 s, MaxSyncAction=500, MaxExport=10000, WindowSize=500 |
| `StateTopic` | enum | `router.dlq.state`; partitions=6; `cleanup.policy=compact` |

### 2.3 `internal/dlq/httpapi` — class-based handlers

`Module{Manager, Problems problem.Writer}` with `Mount(r chi.Router)`; one struct per endpoint:
`ListHandler`, `GlobalListHandler`, `SummaryHandler`, `GlobalSummaryHandler`, `DetailHandler`, `RetryHandler`,
`DiscardHandler`, `RestoreHandler` (P2), `ExportHandler`, `StudioHandler`, `JobHandlers` (P2).
Each handler: decode → `RequestValidator` → Manager call → `Presenter` (DTO) → JSON. Errors map through
`ErrorMapper` (domain error → `problem.Details` with `path`).

### 2.4 `internal/dlq/jobs` (P2) — bulk background jobs

`Job`, `JobRepository` (Postgres), `JobRunner` (claims with `FOR UPDATE SKIP LOCKED`, runs in every API instance),
`JobProgress` events published on the monitoring events bus.

### 2.5 Config (`internal/platform/config/keys.go`)

| EnvKey | Default | Values |
| --- | --- | --- |
| `ROUTER_DLQ_STATE_BACKEND` | `kafka` | `kafka`, `memory` |
| `ROUTER_DLQ_RETRY_EXECUTOR` | `in_process` | `in_process`, `preview` (P2) |

## 3. Data model touchpoints

| Store | Use |
| --- | --- |
| `routes` (read) | team_id (RBAC), active_version_id (retry target), name (global list) |
| `route_versions` (read) | `config->errorPolicy->dlqTopic`, `config->source.connectionId`, destination per branch |
| `connections` (read) | DLQ connection spec + `secret_ref`, sensitive flag, `env_tag` |
| `audit_log` (write via middleware) | actions `dlq.retry`, `dlq.discard`, `dlq.restore`, `dlq.export`, `dlq.viewed` (with `masked` flag) |
| Kafka DLQ topic (read) | per route, default `router.dlq.<route-name>` on the source connection |
| Kafka `router.dlq.state` (read/write) | key `routeID:topic:partition:offset`, value `Resolution` JSON, tombstone = un-resolve |
| Redis | `dlq:lock:{ref}` retry locks (TTL 60 s); `dlq:summary:{routeID}` cached counts (TTL 15 s) |
| Postgres `dlq_jobs` (P2, new) | see section 6 |

DLQ connection: `errorPolicy.dlqConnectionId` if set (owned by pipeline-engine schema), else `source.connectionId`.
The DLQ topic name per version is read from config — no new column in `routes`.

## 4. API (under `/api/v1`, platform-core conventions)

All list endpoints: cursor pagination, RFC 7807 errors with `path`. Ids are opaque `Ref` strings.

| Method + path | Phase | Perm | Purpose |
| --- | --- | --- | --- |
| `GET /routes/{id}/dlq` | MVP | route:read | List page |
| `GET /routes/{id}/dlq/summary` | MVP | route:read | Counts |
| `GET /routes/{id}/dlq/{dlqId}` | MVP | route:read (masked per `SampleVisibility`, P2) | Detail |
| `POST /routes/{id}/dlq/retry` | MVP | route:operate | Sync retry (≤ 500) |
| `POST /routes/{id}/dlq/discard` | MVP | route:operate | Mark discarded (≤ 500) |
| `POST /routes/{id}/dlq/restore` | P2 | route:operate | Un-resolve |
| `GET /routes/{id}/dlq/export` | MVP | route:operate | File download |
| `POST /routes/{id}/dlq/{dlqId}/studio` | MVP | route:edit | Create sample session for Studio |
| `GET /dlq` | MVP | (filtered by readable routes = all teams' routes, open read) | Global list |
| `GET /dlq/summary` | MVP | — | Global badge counts |
| `POST /routes/{id}/dlq/jobs` | P2 | route:operate | Bulk retry/discard job |
| `GET /dlq/jobs/{jobId}`, `POST /dlq/jobs/{jobId}/cancel` | P2 | job owner / admin | Job status, cancel |

### 4.1 List

`GET /routes/{id}/dlq?limit=50&cursor=…&errorType=step_error,timeout&since=2026-10-13T00:00:00Z&until=…&step=…&version=4&q=reduce&includeResolved=false`

```json
{
  "items": [{
    "id": "cm91dGVyLmRscS5vcmRlcnN8MHw4ODQxNjI",
    "routeId": "r3", "routeName": "Order totals → analytics",
    "failedAt": "2026-10-13T09:41:07Z", "errorType": "step_error",
    "message": "Cannot read properties of undefined (reading 'reduce')",
    "step": "Step 1 · JavaScript", "branch": "main", "versionNo": 4,
    "source": { "topic": "order-events", "partition": 0, "offset": 884162 },
    "resolution": null
  }],
  "nextCursor": "eyJ2IjoxLCJwIjp7…",
  "scanned": 1200, "truncated": false
}
```
`truncated=true` means the scan hit a time/byte limit before filling the page; `nextCursor` continues the scan.
`GET /dlq` returns the same shape plus `routeId` filter (multi), merged newest-first across routes.

### 4.2 Summary

```json
{ "unresolved": 3, "byErrorType": { "step_error": 2, "timeout": 1 }, "oldestAt": "…", "newestAt": "…", "approximate": true }
```
`unresolved = Σ(highWatermark − lowWatermark) − resolved still in retention`; flagged `approximate` (compaction, transactions).
`GET /dlq/summary` → `{ "unresolved": 7, "routes": [{ "routeId": "r3", "unresolved": 3 }] }`.

### 4.3 Detail

```json
{
  "id": "…", "routeId": "r3", "failedAt": "…", "errorType": "step_error", "message": "…",
  "stack": "TypeError: …\n    at transform (step-1.js:7:22)", "step": "Step 1 · JavaScript",
  "branch": "main", "versionNo": 4, "source": { "topic": "order-events", "partition": 0, "offset": 884162 },
  "key": { "encoding": "json", "data": "ord_123" },
  "value": { "encoding": "json", "data": { "orderId": "ord_123", "items": null } },
  "headers": [{ "key": "traceId", "value": "ab12", "encoding": "utf8" }],
  "masked": false, "resolution": null
}
```
`encoding` ∈ `json | utf8 | base64` (P2: `avro | protobuf` decoded to JSON with `schemaId`).

### 4.4 Retry / discard / restore

Request (exactly one of `ids` / `match`):
```json
{ "ids": ["…", "…"] }
{ "match": { "errorType": ["step_error"], "since": "…", "until": "…" }, "expectedCount": 3 }
```
Retry response `200`:
```json
{
  "activeVersionNo": 5,
  "results": [
    { "id": "…", "outcome": "sent", "destination": { "topic": "orders-public", "partition": 2, "offset": 77120 } },
    { "id": "…", "outcome": "filtered" },
    { "id": "…", "outcome": "failed", "errorType": "step_error", "message": "…" },
    { "id": "…", "outcome": "skipped", "reason": "locked" }
  ],
  "counts": { "sent": 1, "filtered": 1, "failed": 1, "skipped": 1 },
  "remaining": 0
}
```
`skipped` reasons: `locked`, `already_resolved`, `expired` (offset below low watermark).
Discard response: `{ "discarded": 2, "skipped": [...], "remaining": 0 }`. Restore: `{ "restored": 1 }`.

### 4.5 Export

`GET /routes/{id}/dlq/export?ids=a,b` or same filters as list, `&format=json|ndjson` →
`Content-Disposition: attachment; filename="dlq-<route>-<ts>.json"`; body = array of §4.3 detail objects, streamed.
Over `MaxExport` → `422` problem with `path: "query"`.

### 4.6 Open in Studio

`POST /routes/{id}/dlq/{dlqId}/studio` → `{ "sessionId": "ses_…", "sampleIndex": 0, "builderUrl": "/routes/r3/edit?step=transform&session=ses_…&sample=0" }`.

### 4.7 Jobs (P2)

`POST /routes/{id}/dlq/jobs {"action":"retry","match":{…}}` → `202 { "jobId": "…" }`;
`GET /dlq/jobs/{id}` → `{ "status": "running", "processed": 1200, "total": 5400, "counts": {…}, "startedAt": "…" }`.

### 4.8 Error cases

| Case | Status | `type` / `path` |
| --- | --- | --- |
| Route unknown or not readable | 404 | not-found |
| Missing permission for action/raw | 403 | forbidden |
| Bad id / cursor / filter | 400 | validation, `path` = `/ids/2` / `/cursor` / `/query/since` |
| Both `ids` and `match`, or neither | 400 | validation, `path` = `""` |
| Selection > 500 (sync) | 422 | validation, `path` = `ids` (P2 hint: use jobs) |
| `expectedCount` mismatch (list changed) | 409 | conflict |
| Route has no active version (never deployed / archived) | 409 | conflict, `path` = `activeVersion` |
| DLQ connection unreachable / auth | 502 | upstream, detail per UI §12 error pattern |
| DLQ topic missing | 200 with empty items + `warnings: ["dlq_topic_missing"]` |
| Destination permanent error on retry | 200, per-item `failed` with `errorType=destination` |
| Scan budget hit | 200 with `truncated: true` |

## 5. Flows

### 5.1 List dead letters of a route

```
HTTP GET /api/v1/routes/{id}/dlq?errorType=…&since=…&cursor=…
└─ ListHandler.ServeHTTP
   └─ RequestValidator.Query → invalid limit/cursor/filter → 400 problem with path
   └─ rbac.Authorizer.Check(route:read) → denied → 404 (hide existence)
   └─ Manager.List(routeID, Query)
      └─ Locator.Locate(routeID) → reads route_versions dlqTopic set + DLQ connection
         └─ no version ever deployed → empty page
      └─ KafkaReader.Scan(Location, Query)
         └─ pkg/kafka.Factory.Client(dlqConn) → manual assignment, no consumer group
         └─ since/until set → ListOffsets by timestamp → narrow [start,end) per partition
         └─ cursor set → end offsets = cursor upper bounds
         └─ read windows backwards (WindowSize per partition) → record.Codec.Decode
            └─ decode fails → item with errorType=unknown, message="unreadable DLQ record"
         └─ Query.Matches(record) → keep / skip
         └─ budget (10 s / 10 MiB) exceeded → stop, truncated=true
      └─ StateStore.Lookup(refs) → resolved & !includeResolved → drop
      └─ merge partitions by failedAt desc → take Limit → CursorCodec.Encode(next bounds)
   └─ ViewPolicy.Present(list fields only — message/step, no payload)
   └─ 200 { items, nextCursor, scanned, truncated }
```
---
### 5.2 Global list (Dead letters screen)

```
HTTP GET /api/v1/dlq?routeId=…&errorType=…&cursor=…
└─ GlobalListHandler.ServeHTTP
   └─ rbac.Authorizer.ReadableRoutes(principal) → route ids (filtered by ?routeId)
   └─ Manager.ListGlobal(routes, Query)
      └─ group routes by DLQ connection → one KafkaReader.Scan per (connection, topics) [parallel, bounded 8]
      └─ composite cursor = map routeID → per-route cursor
      └─ k-way merge by failedAt desc → take Limit
      └─ one route scan fails → items from others + warnings[{routeId, reason}]
   └─ 200 { items, nextCursor, truncated, warnings }
```
---
### 5.3 Summary counts (nav badge, tab label, dashboard card)

```
HTTP GET /api/v1/routes/{id}/dlq/summary
└─ SummaryHandler.ServeHTTP
   └─ Redis dlq:summary:{route} hit → 200 cached
   └─ miss → Manager.Summary(routeID)
      └─ KafkaReader.Summary → low/high watermarks per partition of every DLQ topic
      └─ StateStore.CountResolved(route) restricted to offsets ≥ low watermark
      └─ byErrorType → sampled from newest ≤ 1000 records (approximate=true)
      └─ Redis SET dlq:summary:{route} TTL 15 s → 200
```
---
### 5.4 Detail

```
HTTP GET /api/v1/routes/{id}/dlq/{dlqId}
└─ DetailHandler.ServeHTTP
   └─ RefCodec.Decode(dlqId) → invalid → 400 path=/dlqId
   └─ Locator.Locate → ref.Topic ∉ Location.Topics → 404
   └─ KafkaReader.Fetch([ref])
      └─ offset < low watermark → 404 "expired by DLQ retention"
   └─ record.Codec.Decode → engine codec (json MVP; avro/protobuf via schema registry P2) → key/value View
      └─ codec fails → encoding=base64
   └─ StateStore.Lookup → resolution
   └─ ViewPolicy.Present(principal, conn, record)
      └─ P2: SampleVisibility → Viewer, or sensitive connection → type-preserving masked values, masked=true
      └─ otherwise → raw values
   └─ 200 detail
```
---
### 5.5 Retry (single, selected, or "retry all" ≤ 500)

```
HTTP POST /api/v1/routes/{id}/dlq/retry { ids | match, expectedCount? }
└─ RetryHandler.ServeHTTP
   └─ RequestValidator.Selection → both/neither → 400
   └─ rbac.Authorizer.Check(route:operate) → denied → 403
   └─ ActionManager.Apply(ActionRetry, routeID, Selection)
      └─ load route → active_version_id null → 409 no active version
      └─ Selector.Resolve(Selection) → ids → Reader.Fetch ; match → Reader.Scan (unresolved only)
         └─ count > MaxSyncAction → 422 (P2: suggest jobs)
         └─ expectedCount set ∧ ≠ count → 409 conflict
      └─ StateStore.Lookup → already resolved → outcome skipped(already_resolved)
      └─ RetryLock.Acquire(refs) → busy refs → outcome skipped(locked)
      └─ ExecutorRegistry[config].Execute(RetryTarget{activeVersion}, records)
         └─ PipelineCache.Get(versionID) → miss → engine.Compiler.Compile(config) → cache
            └─ compile error → all failed(errorType=compile) — nothing marked
         └─ per record, partition order kept: Decode → Pipeline.Process(event)
            └─ branch header set (P2) → keep outs where Out.Branch = failure.Branch
            └─ zero outs → outcome filtered
            └─ step/timeout/output error → outcome failed(new error), stays unresolved
            └─ outs → producer(destination conn, idempotent, acks=all).ProduceSync
               └─ ack → outcome sent(destRef)
               └─ transient error → retry with backoff ≤ 3 attempts → still failing → failed(destination)
               └─ permanent error → failed(destination) for this and remaining records of that destination
      └─ StateStore.Mark(sent → retried, filtered → filtered) → write router.dlq.state (acks=all)
         └─ state write fails → response still lists outcomes, warning state_not_saved (event may show again)
      └─ RetryLock.Release
      └─ Redis DEL dlq:summary:{route}
      └─ audit dlq.retry {counts, ids|match, versionNo} → audit_log
      └─ metrics router_dlq_actions_total{action=retry,outcome}
   └─ 200 { results, counts, remaining }
```
---
### 5.6 Discard

```
HTTP POST /api/v1/routes/{id}/dlq/discard { ids | match }
└─ DiscardHandler.ServeHTTP
   └─ validate + rbac(route:operate) → errors as 5.5
   └─ ActionManager.Apply(ActionDiscard, routeID, Selection)
      └─ Selector.Resolve → > MaxSyncAction → 422
      └─ DiscardAction.Apply → StateStore.Mark(state=discarded, by, at) → router.dlq.state
      └─ Redis DEL dlq:summary:{route}
      └─ audit dlq.discard {count, ids|match}
   └─ 200 { discarded, skipped, remaining }
```
---
### 5.7 Restore (P2)

```
HTTP POST /api/v1/routes/{id}/dlq/restore { ids }
└─ RestoreHandler.ServeHTTP → rbac(route:operate)
   └─ ActionManager.Apply(ActionRestore)
      └─ resolution.state ≠ discarded → skipped(not_discarded)
      └─ offset < low watermark → skipped(expired)
      └─ StateStore.Clear(refs) → tombstones in router.dlq.state → audit dlq.restore
   └─ 200 { restored }
```
---
### 5.8 Export

```
HTTP GET /api/v1/routes/{id}/dlq/export?ids=…|filters&format=json
└─ ExportHandler.ServeHTTP → rbac(route:operate)
   └─ ExportManager.Export(routeID, Selection, format)
      └─ ExportFormat unknown → 400 path=/format
      └─ Selector count > MaxExport → 422
      └─ set Content-Type/Disposition → stream: Reader iterator → ViewPolicy.Present → Exporter.Write
         └─ client disconnects → stop scan, log only (no payload)
      └─ audit dlq.export {count, format, masked}
```
---
### 5.9 Open in Transform Studio

```
User clicks "Open in Transform Studio" (detail panel)
└─ FE DeadLetterActions.openInStudio(routeId, id)
   └─ HTTP POST /api/v1/routes/{id}/dlq/{dlqId}/studio
      └─ StudioHandler → rbac(route:edit) → denied → 403
      └─ Reader.Fetch → record → ViewPolicy.Present (masked for viewers)
      └─ StudioLinker.Link → live-preview POST /preview/samples {source:"inline", events:[…]} → sessionId
         └─ preview unavailable → 503 problem "Preview service unavailable, try again"
      └─ 200 { sessionId, sampleIndex, builderUrl }
   └─ router.navigate(builderUrl) → builder opens transform step with this sample selected
```
---
### 5.10 Resolved-state index (CompactedTopicStore)

```
[async] first Lookup/Mark for DLQ connection C on this API instance
└─ CompactedTopicStore.ensure(C)
   └─ admin: router.dlq.state missing → CreateTopic(compact, 6 partitions) → not allowed → 502 "cannot create router.dlq.state"
   └─ consume all partitions from earliest → map[key]Resolution (tombstone → delete)
   └─ reached high watermarks → index ready (Lookups block until ready, max 5 s → 503)
   └─ [async] tail goroutine keeps applying new records from other API instances
      └─ client error → backoff reconnect; index marked stale; Lookup still served
---
StateStore.Mark(resolutions)
└─ produce key=routeID:topic:partition:offset value=Resolution (acks=all, idempotent)
└─ ack → apply to local index immediately (read-your-writes on this instance)
```
---
### 5.11 Bulk job (P2)

```
HTTP POST /api/v1/routes/{id}/dlq/jobs {action, match}
└─ JobHandlers.Create → rbac(route:operate) → INSERT dlq_jobs(status=queued) → 202 {jobId}
---
[async] JobRunner tick (every 2 s, each API instance)
└─ SELECT … FROM dlq_jobs WHERE status='queued' FOR UPDATE SKIP LOCKED LIMIT 1 → status=running
   └─ loop pages of ≤ 500 via Selector(match, cursor) → ActionManager.Apply (same path as 5.5/5.6)
      └─ UPDATE dlq_jobs processed/counts/cursor (resumable after crash)
      └─ status=cancel_requested → status=cancelled → stop
   └─ done → status=succeeded → monitoring events bus "dlq.job.finished" → bell + toast
   └─ unrecoverable error → status=failed, error → events bus
---
[cron] JobRunner reaper (every 60 s)
└─ running ∧ heartbeat_at < now − 60 s → status=queued (another instance resumes from cursor)
```
---
### 5.12 State janitor (P2)

```
[cron] leader-only, daily
└─ for each DLQ connection → for each state key
   └─ ref.offset < DLQ topic low watermark → produce tombstone (keeps router.dlq.state small)
```

## 6. Migrations

- MVP: none. `router.dlq.state` is a Kafka topic created on demand (5.10).
- P2: `internal/store/migrations/00NN_dlq_jobs.up.sql` / `.down.sql` (P2 number ≥ 0010, next free at implementation):
  `CREATE TYPE dlq_job_status AS ENUM ('queued','running','cancel_requested','cancelled','succeeded','failed');`
  `CREATE TYPE dlq_job_action AS ENUM ('retry','discard');`
  `dlq_jobs(id uuid pk, route_id uuid fk routes on delete cascade, action dlq_job_action, selection jsonb, status dlq_job_status,
  total int, processed int, counts jsonb, cursor text, error text, requested_by uuid fk users, heartbeat_at, created_at, finished_at)`;
  index `(status) WHERE status IN ('queued','running')`.

## 7. Frontend

Existing: `web/src/domain/deadLetter.ts`, `web/src/mocks/deadLetters.ts`, `web/src/api/contracts.ts#DeadLettersService`,
`web/src/api/mock/MockDeadLettersService.ts`, `web/src/store/DeadLetterActions.ts`, `RouteDetailTab.Dead`.

### 7.1 Domain + contract changes

```ts
export const DeadLetterErrorType = { Decode: 'decode_error', Step: 'step_error', Timeout: 'timeout', OutputInvalid: 'output_invalid' } as const;
export const RetryOutcome = { Sent: 'sent', Filtered: 'filtered', Failed: 'failed', Skipped: 'skipped' } as const;
export type DeadLetter = { id; routeId; routeName; failedAt; errorType; message; step; branch; versionNo; source: SourcePosition; resolution: Resolution | null };
export type DeadLetterDetail = DeadLetter & { stack: string; key: EncodedValue; value: EncodedValue; headers: EncodedHeader[]; masked: boolean };
export type DeadLetterQuery = { routeId?: string; errorTypes?: DeadLetterErrorType[]; since?: string; until?: string; text?: string; includeResolved?: boolean; cursor?: string; limit?: number };
export type DeadLetterSelection = { ids: string[] } | { match: DeadLetterQuery; expectedCount?: number };

export abstract class DeadLettersService {
  abstract list(q: DeadLetterQuery): Promise<Page<DeadLetter>>;
  abstract summary(routeId?: string): Promise<DeadLetterSummary>;
  abstract get(routeId: string, id: string): Promise<DeadLetterDetail>;
  abstract retry(routeId: string, sel: DeadLetterSelection): Promise<RetryReport>;
  abstract discard(routeId: string, sel: DeadLetterSelection): Promise<DiscardReport>;
  abstract exportUrl(routeId: string, sel: DeadLetterSelection, format: ExportFormat): string;
  abstract openInStudio(routeId: string, id: string): Promise<StudioLink>;
}
```
The current `DeadLetterKind` labels map to `DeadLetterErrorType` through a `DeadLetterErrorTypeLabel` record.

### 7.2 Components

| File | Responsibility |
| --- | --- |
| `pages/dead-letters/DeadLettersPage.tsx` (+ `.module.css`) | Global screen: header, filters, list + detail split (design `showDL`) |
| `pages/dead-letters/DeadLetterBrowser.tsx` | Shared list/detail split used by page and route tab; props `routeId?` |
| `pages/dead-letters/components/DeadLetterList.tsx` | Rows: time, error type (red), mono message, route · p@offset; checkbox selection; "Load more" |
| `pages/dead-letters/components/DeadLetterToolbar.tsx` | Count, error-type chips, time range, Retry all / Discard selected / Export |
| `pages/dead-letters/components/DeadLetterDetail.tsx` | Title type · step, stack `<pre>`, original event JSON tree, action buttons |
| `pages/dead-letters/components/RetryReportDialog.tsx` | Per-outcome counts + failed items with new errors |
| `pages/dead-letters/components/DeadLetterEmptyState.tsx` | "No dead letters. Failed events land here with the reason…" |
| `pages/dead-letters/DeadLetterBrowserState.ts` | Class: query, cursor, selection, selected id; pure transitions (unit-tested) |
| `pages/route-detail/tabs/DeadLettersTab.tsx` | `<DeadLetterBrowser routeId=…/>`; tab label `Dead letters · N` from summary |
| `api/http/HttpDeadLettersService.ts` | REST implementation registered as `DataSource.Http` |

### 7.3 FE flow — retry selected

```
User selects rows → clicks "Retry"
└─ DeadLetterActions.retry(routeId, {ids})
   └─ services.deadLetters.retry → RetryReport
      └─ all sent/filtered → remove rows, toast "Retried N events · sent"
      └─ some failed → keep failed rows, update message, open RetryReportDialog
      └─ some skipped(locked) → toast "N events are being retried by someone else"
      └─ 409 no active version → toast "Deploy the route before retrying" + link to builder
   └─ AppStore summary refresh → nav badge + tab label update
```

## 8. Security

- Route-scoped RBAC on every endpoint; DLQ connection credentials resolved via `internal/secrets.Manager` only.
- Masking applies to detail, export and studio session: Viewers always, other roles on sensitive connections (P2, full roles).
- No payload in logs, metrics labels or audit diffs (audit stores ids/counts only).

## 9. Observability

`router_dlq_api_scan_duration_seconds` (histogram), `router_dlq_api_scan_records_total`, `router_dlq_actions_total{action,outcome}`,
`router_dlq_state_index_ready{connection}` (gauge). Logs carry `route_id`, `dlq_topic`, never payloads.

## 10. Decisions

1. Retry runs an engine call (no preview service) inside the API process in MVP (`in_process` executor). The `preview`
   executor is an optional P2 alternative.
2. The global screen covers every route the user may read. Read access is open across teams (access-control
   decision), so this means all teams' routes. Masking still applies.
3. DLQ topic retention is 7 days. route-runtime sets it when it creates the topic.
4. A retried event that fails again is only reported in the retry report. No new DLQ record is written.
5. The DLQ connection is `errorPolicy.dlqConnectionId` (added to the schema by pipeline-engine) and defaults to the source connection.
6. Masking: Viewers are always masked. Other roles are masked only on connections marked `sensitive`. Masks keep types.
7. The dead-letters tab is registered in the route-detail shell owned by routes (`routes#m-fe-detail-shell`).

## 11. P2 implementation notes (as built)

- Record contract: optional header `router.delivered_branches` (`record.BranchSet`, comma list). Branch-aware retry
  skips branches listed there instead of keeping only `failure.Branch`: the worker delivers all-or-nothing today
  (engine `Process` fails the whole record), so every branch must be re-emitted until route-runtime writes the header.
- Masking: `MaskingViewPolicy` = `rbac.SampleAccess` (surface `dlq`, fails closed) + the feature's `TypePreservingMasker`
  until live-preview's MaskerManager (LP-105) exists. Detail, export and the studio upload use it; detail views are audited.
- Schema payloads: `SchemaPayloads` decodes avro/protobuf with pkg/engine codecs over the source connection registry
  (`codec.HTTPRegistryProvider` + live-preview `SpecRegistryConfigs`); the retry `PipelineCache` compiles with the same provider.
- Jobs live in `internal/dlq/jobs` (repository, runner, notifier, service); migration `1100_dlq_jobs`; job store
  `ROUTER_DLQ_JOB_STORE=postgres|memory`. `dlq.job.*` events go through a `Publisher` port (monitoring bus, not built yet;
  `LogNotifier` wired). The web app polls the job and adds a top-bar Alerts entry when it ends.
- Feature-owned audit actions `dlq.restored`, `dlq.job_created`, `dlq.job_cancelled` are registered in the access-control
  catalog at wiring time.
- Janitor: `StateJanitor`, daily, leader via `scheduler.PgLeaderElector` with advisory lock 7110.

### 11.1 Branch-aware retry

```
RetryRun.One(record)
└─ Pipeline.Process → outs (every branch)
   └─ BranchScope.Keep(failure, outs)
      └─ router.delivered_branches empty → keep every branch (worker delivered nothing)
      └─ branch listed as delivered → drop its outs (no duplicate on a healthy branch)
   └─ zero outs left → outcome filtered
   └─ outs → encode → produce per destination connection → outcome sent
```
---
### 11.2 Bulk job (as built)

```
HTTP POST /api/v1/routes/{id}/dlq/jobs {action, match}
└─ CreateJobHandler → Manager.PrepareJob (retry|discard only → else 400; retry without active version → 409)
   └─ PostgresRepository.Create → dlq_jobs(status=queued) → 202 job; audit dlq.job_created
---
[async] jobs.Runner.Tick (every 2 s, each API instance)
└─ Claim (UPDATE … WHERE id = (SELECT … FOR UPDATE SKIP LOCKED)) → running
   └─ first run → Manager.CountMatches → total
   └─ loop Manager.ActPage(cursor) → page ≤ 500 unresolved below cursor → ActionHandler.Apply (resolutions carry jobId)
      └─ Progress(processed, counts, cursor) → stored status cancel_requested → Finish(cancelled) → notify
      └─ page.Done → Finish(succeeded) → notify (LogNotifier; BusNotifier when the events bus exists)
      └─ ActPage error → Finish(failed, error) → notify
      └─ Progress write fails (crash) → job stays running → reaper requeues; resumed run starts at the stored cursor,
         already-resolved records of the lost page are filtered by the scan (never acted on twice)
---
[cron] jobs.Runner.Reap (every 60 s) → running ∧ heartbeat older than 60 s → queued
```
---
### 11.3 Restore

```
HTTP POST /api/v1/routes/{id}/dlq/restore {ids | match}
└─ RestoreHandler → Manager.Act(ActionRestore) (match selections include resolved records)
   └─ Selector → expired offsets → skipped(expired)
   └─ RestoreAction: StateGate.Lookup → not discarded → skipped(not_discarded)
      └─ StateGate.Clear → tombstones in router.dlq.state → outcome restored; audit dlq.restored
```

## 12. P3 implementation notes (as built)

No migration. New settings: `ROUTER_DLQ_DEMO_SEED=seed|none` (default `seed`). New audit action `dlq.job_finished`
(feature-owned, registered in the catalog at wiring like the P2 actions). API additions (additive, OpenAPI updated):
list/detail rows carry `deliveryMode` (active version's `delivery.mode`, absent for undeployed routes); the retry report
carries `deliveryMode` and `deliveredAtLeastOnce`; the retry audit diff carries `deliveredAtLeastOnce` when true.

### 12.1 Demo dead letters (FR-20, DL-201/205)

The demo cluster (`kind=demo`, connections CN-201) lives inside each process, so the API seeds the DLQ topic of a demo
route on its own cluster the first time the route is located (list, summary, detail, actions). Seeding is idempotent:
a topic that ever held a record is left alone; a restarted API (empty in-memory cluster) seeds again.

```
HTTP GET /api/v1/routes/{id}/dlq (or summary/detail/actions/global list)
└─ DemoSeedingLocator.Locate → StoreLocator.Locate (targets of all versions)
   └─ KafkaDemoSeeder.Ensure (selected by ROUTER_DLQ_DEMO_SEED; NoDemoSeeder when none)
      └─ target connection kind ≠ demo → skip
      └─ topic already seeded in this process → skip
      └─ FranzSource.Ranges(topic) → any partition high > 0 → mark seeded, skip
      └─ empty or missing → DemoTargetOf(active version: first JS step, branch, source topic)
         └─ DemoDeadLetterSet.Records → 12 records (5 step_error items=null, 3 output_invalid, 2 timeout, 2 decode_error)
            spread over the last 2 h → record.Codec.Encode → produce (auto-create topic) → mark seeded
      └─ seeding error → warn log dlq.demo_seed_failed → the read continues unseeded
```

Demo connections are opened through `SpecClientOpener.Kinds[demo]` = `SandboxKindOptions{DemoClientFactory}` (the
process-wide pool, auto topic creation on), so browsing, retry (destination produce) and the state store all work on
the demo cluster. FE: the first-run checklist shows `DemoDeadLettersLink` when a route reads a demo connection.

### 12.2 Exactly-once routes (FR-21, DL-202/204/207)

```
HTTP POST /api/v1/routes/{id}/dlq/retry
└─ Manager.Act → retryTarget (active version)
   └─ RetryAction.Apply → report.Delivery = RetryDeliveryOf(target) (mode; downgraded = RetryDowngrades[mode])
      └─ InProcessExecutor → FranzProducer (idempotent, acks=all, no transactional id) → outside the route transaction
   └─ RetryDTO {deliveryMode, deliveredAtLeastOnce} → audit dlq.retried {…, deliveredAtLeastOnce}
---
FE detail of a row with deliveryMode=exactly_once → AtLeastOnceNotice
FE retry report deliveredAtLeastOnce → RetryReportDialog opens (even without failures) with AtLeastOnceNotice; toast suffix
FE "Retry all" over-cap BulkJobDialog on an exactly_once route → AtLeastOnceNotice
```

### 12.3 Audit export / SIEM (DL-203/206)

Synchronous actions already write `audit_log` rows with object type `dlq` (route id/name label/team). Background jobs ran
without a request scope, so their effect was only visible as `dlq.job_created`; they now end with:

```
[async] jobs.Runner.finish → Notifiers (log, bus, AuditNotifier)
└─ AuditNotifier.JobEnded → Manager.Route (label/team; job's stored route name on error)
   └─ RecorderAuditTrail (actor kind system) → audit.Recorder → audit_log dlq.job_finished
      {jobId, action, status, processed, counts, match, requestedBy, error} → audit export (JSONL/CSV) + stream sinks
```

FE: audit page preset chip "Dead-letter actions" (`objectType=dlq`, also `/audit?preset=dlq`; Dead letters header link
"Audit trail →" for `audit.read`).

### 12.4 Performance gate (DL-208)

`TestListP95OnMillionRecordTopic` (kfake, 1 M records over 7 days in 6 partitions; skipped with `-short`, size via
`ROUTER_DLQ_PERF_RECORDS`) fails when list p95 > 1.5 s over time-filtered pages (last hour, a one-hour window a day back
with an error-type filter, 3 days with page 200, plus next pages). Measured: p50 96 ms, p95 613 ms.
