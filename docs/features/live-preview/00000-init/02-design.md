# Live Preview — Design (00000-init)

Owns: `cmd/preview`, `internal/preview/**`, `pkg/kafka/sampling*.go` (sampling helpers only),
the API-side preview gateway (`internal/preview/gateway`, mounted by `cmd/api`), and the web sample
panel (`web/src/components/sample-panel`, `web/src/domain/preview`, `PreviewService` contract).
Cites, does not redefine: route config schema + trace model (pipeline-engine), sandbox (js-sandbox),
Kafka client factory + `sensitive` flag (connections), auth/problem/audit/Redis (platform-core),
permissions (access-control).

## 1. Architecture

```
Browser (sample panel)
  └─ HTTPS /api/v1/preview/*  (OIDC session cookie)
       └─ cmd/api · internal/preview/gateway
            ├─ Authorizer (access-control)  → team may use connection? caller has samples.read_raw?
            ├─ AuditRecorder (platform-core) → audit_log row per load / raw view
            └─ PreviewClient ── mTLS /internal/v1/preview/* + CallerDirective header
                 └─ cmd/preview · internal/preview
                      ├─ SampleSourceManager → KafkaLatest | KafkaTimeRange | Upload | Demo
                      │    └─ pkg/kafka PartitionSampler (manual assignment, no group)
                      ├─ SessionStoreManager  → RedisSessionStore | MemorySessionStore
                      ├─ SchemaInferrer
                      ├─ PreviewRunner → pkg/engine Compile/Process(trace) + pkg/sandbox
                      │    └─ TraceEncoder (PatchDiffer RFC 6902) + StatusClassifier
                      ├─ ResponseMasker (MaskerManager)  — only when directive says masked
                      └─ TestTopicWriter + TestTopicJanitor [cron]
```

Decision: the API gateway decides **whether** to mask (it owns identity and RBAC); the preview
service performs the masking on its response so raw values never leave it for masked callers
(SysDesign §10 "masked by the API before they reach the browser" is satisfied at the boundary the
API controls). The directive is an internal header only accepted over mTLS from the API.

## 2. Go packages and key types

### pkg/kafka (sampling helpers; client factory owned by connections)
```go
type PartitionWindow struct { Topic string; Partition int32; Start, End int64 } // End = HWM snapshot (exclusive)
type OffsetPlanner interface {                         // pure, unit-tested
    Plan(ctx context.Context, plan SamplingPlan, marks []PartitionMarks) []PartitionWindow
}
type LatestOffsetPlanner struct{}   // start = max(low, high - ceil(N/partitions))
type TimeRangeOffsetPlanner struct{} // start = ListOffsets(from); stop when ts > to or End reached
type PartitionSampler interface {    // kgo.ConsumePartitions, no group, no commits
    Read(ctx context.Context, windows []PartitionWindow, budget Budget) ([]RawRecord, StopReason, error)
}
type StopReason string // enum: complete | max_events | max_bytes | deadline
```

### internal/preview (domain, used by cmd/preview)
```go
type SampleMode string   // enum: latest | time_range | upload | demo
type EventStatus string  // enum: passed | filtered | error | dropped
type SampleLimit int64   // domain constants: MaxEvents=1000, MaxBytes=10<<20, DefaultCount=50
type SessionPolicy time.Duration // SessionTTL=1h, LoadDeadline=10s, RunDeadline=10s, TestTopicTTL=24h

type SessionID string
type Session struct {
    ID SessionID; OwnerID, TeamID, ConnectionID string; Topics []string
    Mode SampleMode; Format engine.FormatType; Loaded int; Bytes int64
    Truncated bool; StopReason kafka.StopReason; CreatedAt, ExpiresAt time.Time
}
type SampleRecord struct {          // raw bytes kept so format overrides re-decode
    Seq int; Topic string; Partition int32; Offset int64; Timestamp time.Time
    Key, Value []byte; Headers []engine.Header
}

// Sample sources — base interface + Manager; selected by SampleMode registry (no switch).
type SampleSource interface {
    Mode() SampleMode
    Load(ctx context.Context, req LoadRequest, budget *SampleBudget) (LoadResult, error)
}
type SampleSourceManager struct{ sources map[SampleMode]SampleSource }

// Session stores — selected by config key ROUTER_PREVIEW_SESSION_STORE (redis | memory).
type SessionStore interface {
    Create(ctx context.Context, s Session, recs []SampleRecord) error
    Get(ctx context.Context, id SessionID) (Session, error)          // ErrSessionExpired
    Records(ctx context.Context, id SessionID, page Page) ([]SampleRecord, Cursor, error)
    All(ctx context.Context, id SessionID) ([]SampleRecord, error)
    PutSchema(ctx context.Context, id SessionID, fs FieldSchema) error
    Touch(ctx context.Context, id SessionID) error                    // slide TTL on use
    Delete(ctx context.Context, id SessionID) error
}
type SessionStoreManager struct{ active SessionStore }

type SampleBudget struct{ /* events, bytes, deadline; Admit(rec) bool; Reason() */ }
type SampleSearcher struct{}  // case-insensitive text match over key, decoded value, headers

type SchemaInferrer struct{ topValues, lowCardinality int } // 20, 50 distinct
type FieldNode struct {
    Path string; Types map[JSONType]int; Present int; Frequency float64
    TopValues []ValueCount; Children []FieldNode
}
type FieldSchema struct{ Root FieldNode; Samples int }

// Preview run
type RunRequest struct {
    SessionID SessionID; Events []InlineEvent; Config json.RawMessage
    EventSeqs []int; Mode RunMode   // RunMode enum: preview | test
}
type PreviewRunner struct {
    compiler engine.Compiler; codecs engine.CodecManager
    encoder TraceEncoder; classifier StatusClassifier; validator OutputValidator
}
type TraceEncoder interface{ Encode(input engine.Event, t *engine.Trace) TraceDTO }
type PatchDiffer interface{ Diff(prev, next any) []PatchOp } // emits add | remove | replace only
type StatusClassifier struct{} // trace + outs + err → EventStatus per branch

// Masking — base + Manager; MaskStyle enum: redact | partial | type_preserving
type ValueMasker interface{ Style() MaskStyle; Mask(v any) any }
type MaskerManager struct{ maskers map[MaskStyle]ValueMasker; defaultStyle MaskStyle }
type ResponseMasker struct{ /* walks SampleDTO/RunResultDTO incl. patch values, headers, raw text */ }

// Test topic (P2)
type TestTopicWriter struct{ admin kafka.AdminFactory; producers kafka.ProducerFactory; reg TestTopicRegistry }
type TestTopicRegistry interface{ Record(...); Expired(ctx, now) ([]TestTopic, error); MarkDeleted(...) }
type TestTopicJanitor struct{ /* Redis lock "preview:janitor" so one replica runs it */ }
```

### internal/preview/gateway (mounted in cmd/api)
Class-based handlers, one per endpoint: `LoadSamplesHandler`, `ListEventsHandler`,
`SchemaHandler`, `DeleteSessionHandler`, `RunHandler`, `TestRunHandler`. Shared collaborators:
`PreviewAuthorizer` (team/connection + `samples.read_raw` via access-control `Policy`),
`PreviewClient` (internal HTTP, base URL `ROUTER_PREVIEW_URL`), `CallerDirective`
(`{userId, teamId, mask: bool, maskStyle}` signed header `X-Router-Caller`).

### cmd/preview
`previewService.Mount` adds `/internal/v1/preview/*` routes (class handlers in
`internal/preview/httpapi`), concurrency gate `RunGate` (semaphore, `ROUTER_PREVIEW_MAX_RUNS`),
and starts `TestTopicJanitor` (P2). New env keys added to `internal/platform/config/keys.go`:
`ROUTER_PREVIEW_URL`, `ROUTER_PREVIEW_SESSION_STORE`, `ROUTER_PREVIEW_MAX_RUNS`.

## 3. Data model touchpoints

- Redis (sessions, TTL 1 h sliding, hard cap 10 MB):
  `preview:session:{id}` hash (Session JSON) · `preview:session:{id}:records` list (SampleRecord
  msgpack/JSON with base64 bytes) · `preview:session:{id}:schema` string · `preview:janitor` lock.
- Postgres reads: `connections` (security, `sensitive`, team), `routes` (name for test topic),
  `lookup_table_versions`/`transform_versions` via engine compile of library refs.
- Postgres writes: `audit_log` (`samples.view` with `masked` flag, `preview.test_topic.write`).
- **Migration `00NN_preview_test_topics` (P2, number ≥ 0010, next free at implementation):** table `preview_test_topics(id uuid pk, route_id uuid
  null fk routes, connection_id uuid fk connections, topic text, created_by uuid fk users,
  created_at, expires_at, deleted_at null)`, index on `(expires_at) WHERE deleted_at IS NULL`.
- No `connections` migration here: `connections.sensitive` is owned by connections (`0004_connections`, MVP)
  and is only read by this folder.

## 4. API (public, `/api/v1`, via gateway)

`POST /preview/samples` — create a session.
```json
req:  { "connectionId":"uuid", "topics":["order-events"], "mode":"latest",
        "count":50, "from":null, "to":null, "format":"json", "events":null }
      // mode=time_range → from/to RFC3339; mode=upload → events:[{key,value,headers,timestamp}]
resp: { "session": { "id":"ps_…", "expiresAt":"…", "loaded":50, "bytes":81234,
        "truncated":false, "stopReason":"complete", "masked":false },
        "events": [ SampleDTO… first page ], "nextCursor":"…" }
SampleDTO: { "seq":0, "topic":"order-events", "partition":3, "offset":884120,
             "timestamp":"…", "key":…, "value":{…}, "headers":[{"key":"traceId","value":"…"}],
             "raw": { "key":"…", "value":"…", "encoding":"utf8|base64" },
             "decodeError":null }
```
`GET /preview/sessions/{id}/events?cursor=&limit=100&q=text` → `{ events:[SampleDTO], nextCursor }`
`GET /preview/sessions/{id}/schema` → `{ samples:50, root:{ path, types:{string:50}, frequency,
topValues:[{value:"OrderPaid",count:6,pct:0.12}], children:[…] } }`
`DELETE /preview/sessions/{id}` → 204.

`POST /preview/run` — run draft config on session or inline events.
```json
req:  { "sessionId":"ps_…", "events":null, "config":{ RouteConfig draft }, "eventSeqs":null }
resp: { "summary": { "loaded":500, "passed":37, "filtered":461, "errors":2, "dropped":0,
                     "incomplete":false, "durationMs":184 },
        "issues": [ { "path":"/branches/0/filter/children/1/value", "severity":"warning",
                      "message":"coerced \"100\" to number" } ],
        "results": [ { "seq":0, "branches": [ {
            "branch":"main", "status":"filtered",
            "filter": [ { "path":"/branches/0/filter/children/0", "result":true },
                        { "path":"/branches/0/filter/children/1", "result":false,
                          "actual":42, "warning":null } ],
            "steps":  [ { "index":0, "kind":"rename", "patch":[{"op":"add","path":"/value/customerId","value":1040},
                                                               {"op":"remove","path":"/value/cust_id"}],
                          "durationUs":12, "logs":[], "error":null } ],
            "error": null,
            "outputs": [ { "topic":"orders-public", "key":…, "headers":[…], "sizeBytes":412,
                           "valid":true, "validationError":null } ] } ] } ] }
```
Step snapshots: step 0 patch is relative to the decoded input event (`{key,value,headers}`);
step n relative to step n-1. Error results carry `{stepIndex, kind, errorType, message}`.

`POST /preview/test-runs` — same as run with `mode=test` (output validation mandatory) plus
`"writeToTopic": true` (P2) → adds `"testTopic": { "connectionId", "topic":
"router.test.<route>.<yyyymmddhhmmss>", "produced":37, "expiresAt" }`.

Not owned: `POST /preview/infer-mapping` (transform-studio).

## 5. Flows

```
User opens builder / clicks "Load samples" (latest N)
  └─ FE SamplePanel → PreviewService.loadSamples({mode:latest,count})
       └─ POST /api/v1/preview/samples → LoadSamplesHandler
            ├─ PreviewAuthorizer: team may not use connection → 403 problem path=/connectionId
            ├─ P2: access-control SampleVisibility → caller is Viewer, or connection.sensitive → directive.mask=true
            └─ PreviewClient → /internal/v1/preview/samples (mTLS, X-Router-Caller)
                 └─ SampleSourceManager.For(latest) → KafkaLatestSource.Load
                      ├─ secrets + client via connections factory (no group.id) → broker unreachable → 502 problem "Can't reach broker…"
                      ├─ ListOffsets(low/high) per partition → LatestOffsetPlanner → windows
                      ├─ PartitionSampler.Read(windows, budget)
                      │    ├─ budget hit (1000 / 10 MB / 10 s) → stop, Truncated=true, StopReason set
                      │    └─ all windows reached HWM snapshot → StopReason=complete
                      ├─ merge by timestamp, keep latest N
                      └─ SessionStore.Create(session, records) → Redis keys with TTL 1 h
                 ├─ SchemaInferrer.Infer → SessionStore.PutSchema   [async goroutine, bounded]
                 ├─ directive.mask → ResponseMasker on first page
                 └─ 201 { session, events page }
            └─ AuditRecorder → audit_log samples.view {masked: true|false}
```
---
```
User uploads JSON file
  └─ FE parses size (≤10 MB) → POST /preview/samples {mode:upload, events}
       └─ LoadSamplesHandler → authorize (connection optional; team scope from route draft)
            └─ UploadSource.Load
                 ├─ not array/NDJSON → 400 problem path=/events
                 ├─ item invalid → 400 problem path=/events/{i}
                 ├─ > MaxEvents or MaxBytes → truncate, StopReason=max_events|max_bytes
                 └─ synthesize topic/partition=-1/offset=seq → SessionStore.Create → 201
```
---
```
Draft config changes (debounced 300 ms) OR Ctrl+Enter OR "Run" in studio
  └─ FE PreviewRunController: abort in-flight request (AbortController), send new run
       └─ POST /api/v1/preview/run → RunHandler → authorize (session owner == caller, else 404)
            └─ /internal/v1/preview/run → RunGate.Acquire
                 ├─ gate full → 429 problem "Preview is busy, retry in a moment"
                 └─ PreviewRunner.Run (deadline 10 s, ctx cancelled if client disconnects)
                      ├─ SessionStore.Get → expired → 410 problem type=preview/session-expired
                      ├─ engine Compile(config) → issues with error → 422 problem + issues[path]
                      ├─ for each record: CodecManager.Decode
                      │    └─ decode error → status=error errorType=decode (matches worker DLQ rule)
                      ├─ Pipeline.Process(event, trace)
                      │    ├─ filter false → status=filtered, filter[] node results
                      │    ├─ step returns [] → status=dropped at stepIndex
                      │    ├─ step error / JS timeout (js-sandbox limits) → status=error
                      │    └─ outs → Encode + OutputValidator
                      │         ├─ invalid → status=error errorType=output_invalid
                      │         └─ ok → status=passed, outputs[]
                      ├─ TraceEncoder: per-step PatchDiffer.Diff(prev, next)
                      ├─ deadline hit → summary.incomplete=true, unprocessed seqs omitted
                      ├─ directive.mask → ResponseMasker (values, patches, outputs, raw)
                      └─ 200 RunResult; SessionStore.Touch (slide TTL)
  └─ FE: store result → SamplePanel projects view for current BuilderStep
```
---
```
User clicks a filtered event ("Why was this filtered?")
  └─ SamplePanel.selectEvent(seq) → PreviewStore.whyFiltered = result.filter[]
       └─ FilterBuilder (routes) reads node results by JSON pointer
            ├─ result=true  → row green
            ├─ result=false → row red + tooltip "actual: 42"
            └─ node not evaluated (short-circuit) → row grey "not evaluated"
```
---
```
User selects a builder step (source / filter / transform step k / destination)
  └─ StepProjection.project(result, step)
       ├─ source       → decoded input (session event), status hidden
       ├─ filter       → input + status passed|filtered|error(decode)
       ├─ transform k  → JsonPatchApplier.apply(input, patches[0..k]) ; dropped/error before k → badge
       └─ destination  → outputs[] (key, headers, size, validation) + final status
```
---
```
User clicks Test (optionally "also write to test topic", P2)
  └─ POST /api/v1/preview/test-runs {sessionId, config, writeToTopic}
       └─ authorize: editor on route team; writeToTopic → may write to destination connection?
            └─ PreviewRunner.Run(mode=test) → same as run, output validation mandatory
                 ├─ writeToTopic=false → 200 RunResult
                 └─ writeToTopic=true → TestTopicWriter
                      ├─ admin CreateTopic router.test.<route>.<ts> (partitions=1, retention 24 h)
                      │    └─ authorization denied → 403 problem "Can't create test topic on <conn>…"
                      ├─ produce passed outputs (idempotent, acks=all) — never the real destination topic
                      ├─ TestTopicRegistry.Record → preview_test_topics row
                      └─ 200 RunResult + testTopic ; audit preview.test_topic.write
```
---
```
[cron] every 15 min on each preview replica
  └─ TestTopicJanitor.Tick
       ├─ Redis SETNX preview:janitor (TTL 10 min) fails → skip (another replica runs)
       └─ TestTopicRegistry.Expired(now)
            └─ for each: admin DeleteTopic
                 ├─ ok or unknown topic → MarkDeleted
                 └─ error → keep row, log warn (retry next tick)
```
---
```
User right-clicks a value in the panel
  └─ ValueContextMenu → emits PanelValueAction {kind, path, value}
       ├─ kind=keep_only → routes FilterBuilder adds cond row (eq)          (routes owns)
       └─ kind=mask|rename|remove|use_as_key → studio adds recipe step       (transform-studio owns)
```

## 6. Frontend

- `web/src/api/contracts.ts`: new `abstract class PreviewService { loadSamples, listEvents, schema,
  run, testRun, close }`. `SamplesService.list/fields/peek` deprecated in favour of it (connections'
  Peek calls `loadSamples({count: 20})`); tail stays with monitoring.
- `web/src/api/mock/MockPreviewService.ts`: sessions in memory from `mocks/samples.ts`; runs use the
  existing `domain/pipeline` (`FilterEvaluator`, `TransformRunner`) and emit the same DTO shape
  (patches via `JsonPatchDiffer`). `HttpPreviewService` later, registered under `DataSource.Http`.
- `web/src/domain/preview/`: `types.ts` (SampleSession, SampleDTO, RunResult, EventStatus/SampleMode/
  SampleView enums), `JsonPatchApplier.ts` + `JsonPatchDiffer.ts` (add/remove/replace),
  `StepProjection.ts`, `SampleSummary.ts` (counts → "500 loaded · 37 pass · …"),
  `TableColumns.ts` (field paths → columns via FieldSchema).
- `web/src/store/PreviewActions.ts` (+ slice in AppStore): session, last result, view, selected seq,
  run status; `PreviewRunController` handles debounce + abort + 410 auto-reload.
- `web/src/components/sample-panel/`: `SamplePanel.tsx` (collapsible), `SampleToolbar.tsx`
  (load menu: latest N / time range / upload / search), `SummaryBar.tsx`, `PrettyView.tsx`
  (`JsonTree` with type icons), `TableView.tsx`, `RawView.tsx`, `EventStatusBadge.tsx`,
  `EventPager.tsx`, `ValueContextMenu.tsx`, `TestRunDialog.tsx`, CSS Modules + design tokens.
- Shortcut `Ctrl+Enter` registered in `store/shortcuts.ts`.
- As built (MVP): the panel state is per panel instance in `web/src/store/preview/` (`usePreviewSession`), not an
  AppStore slice, so builder and studio sessions never mix. The builder adapts the server run to its existing
  per-sample view with `PipelinePreview.fromRun` (rail counts, match counter, review dry run). `?session=<id>`
  (dead letters "Open in Studio") attaches an existing session instead of loading the source.

```
Builder opens / source connection or topics change
  └─ useBuilderPreview → PreviewActions.ensureLatest(source) (same request already loaded → no-op)
       └─ SampleLoader.load → POST /preview/samples → follow nextCursor pages (≤ 1,000) → GET …/schema [async]
            ├─ failure → panel EmptyState with PanelMessages (connection name, topic, broker)
            └─ ok → store {session, records} → effect schedules a run
---
Draft edit (config JSON changed) or Ctrl+Enter / Test
  └─ PreviewRunController.schedule (300 ms) | runNow → abort in-flight run
       └─ POST /preview/run
            ├─ 410 → SampleLoader.reload → run again once
            ├─ 429 → wait 1 s → run again once → still busy → runFailure "Preview is busy…"
            ├─ 422 → runFailure (message + path) shown in the panel, last result kept
            └─ 200 → store.result → StepProjection per record → panel; PipelinePreview.fromRun → rail / filter / review
  └─ Test → runNow + POST /preview/test-runs → toast + TestRunDialog
```

## 7. Error cases (RFC 7807, `path` where a form field applies)

| Case | Status | type | UI message pattern |
| --- | --- | --- | --- |
| Connection not usable by team | 403 | preview/forbidden | "You can't read samples from <conn>. Ask an admin…" |
| Topic missing | 404 | preview/topic-not-found, path `/topics/0` | "Topic X doesn't exist on <conn>." |
| Broker unreachable / auth fail | 502 | preview/source-unavailable | "Can't reach broker kafka-1:9092. … Test the connection." |
| Empty topic | 201 | — (loaded 0) | "No events yet. Upload a JSON file to test." |
| Limit hit | 201 | — (truncated) | "Showing first 1,000 events (limit)." |
| Session expired / not owner | 410 / 404 | preview/session-expired | silent reload of samples |
| Compile error | 422 | preview/invalid-config + issues[] | inline at path |
| Busy | 429 | preview/busy | toast, auto-retry once |
| Run deadline | 200 | — (incomplete) | "Previewed 640 of 1,000 events (time limit)." |
| Upload malformed / too large | 400 / 413 | preview/invalid-upload | "Line 12 is not valid JSON." |
| Test topic create denied | 403 | preview/test-topic-denied | "Can't create test topic on <conn>…" |

## 8. Testing

- Unit: offset planners (skewed/empty partitions, N < partitions), SampleBudget, SchemaInferrer
  (mixed types, arrays, top-20), PatchDiffer/JsonPatchApplier round-trip (Go ↔ TS golden fixtures),
  StatusClassifier, MaskerManager/ResponseMasker (no raw value survives), SampleSearcher.
- Golden: `internal/preview/testdata/*.json` (config + events → RunResult) shared with pipeline-engine goldens.
- Integration (testcontainers Kafka + Redis): latest/time-range sampling never creates a consumer
  group or commits; limits; session TTL; test topic create + janitor delete.
- Parity: same config + samples through `PreviewRunner` and a real worker runner → identical outputs.
- FE: Vitest for domain/preview; Playwright: load → edit filter → counts change → why-filtered.

## 9. Decisions

1. Test-topic write is P2 (MVP Test runs only on loaded samples).
2. The sample session TTL slides: every run or page read refreshes it to 1 h.
3. The default mask style is type-preserving: strings become a masked string, numbers become 0, and types are
   kept so filter previews stay readable.
4. Who is masked: Viewers always, and other callers only on connections marked `sensitive` (P2,
   access-control `SampleVisibility`). `connections.sensitive` is read-only here.
5. Uploads are allowed without a source connection, for any editor.
6. Peek (connections) reuses `POST /preview/samples` with `count=20`.
7. `ctx.now` is frozen per event. Preview uses the sample timestamp, the same as for test cases.

## 10. Open questions (remaining)

1. Masking executed in the preview service under an API directive (design) vs strictly in the API
   process (spec wording) — acceptable?

## 11. As built (P2)

- New env keys (`internal/preview/config.go`): `ROUTER_PREVIEW_SCHEMA_REGISTRY` (`control_plane` | `none`),
  `ROUTER_PREVIEW_TEST_TOPICS` (`postgres` | `memory`; postgres makes cmd/preview open the metadata DB),
  `ROUTER_PREVIEW_RATE_PER_MINUTE` (240) / `ROUTER_PREVIEW_RATE_BURST` (40).
- New endpoint `PATCH /preview/sessions/{id}` `{format}` → `{session, events, nextCursor}` (format override).
- `SessionDTO.format`, `RunSummary.branches[]`, `RunResult.masked`, `RunResult.testTopic`, `FieldNode.registry`,
  run body `writeToTopic` + optional `routeId`. Audit action `preview.test_topic_written` (object `topic`).
- Migration `1070_preview_test_topics` (number range 1070..1079 of this folder).

```
API gateway builds the caller directive (every load / run / test run)
  └─ masking source (connectionId)
       ├─ load → request.connectionId ; session pages → ""
       ├─ run / test-run with sessionId, or inline run with sampleSessionId (Transform Studio)
       │    └─ GET /internal/v1/preview/sessions/{id} → session.connectionId ("" for uploads)
       │         └─ session missing / foreign / expired → preview problem relayed (404 / 410)
       └─ inline run without a session → draft source.connectionId
  └─ PreviewAuthorizer.Caller(principal, connectionId)
       └─ rbac.SampleAccess.Decide(preview, connectionId)
            ├─ connection unknown (not in the directory) → 404 problem path=/connectionId, nothing forwarded
            ├─ connectionId "" → role only: Viewer → mask = true ; editor / admin → mask = false
            ├─ Viewer, or connection sensitive, or lookup failed → directive.mask = true
            └─ otherwise → mask = false
  └─ preview service: Session.Masked = directive.mask at load (sticky)
       └─ MaskPolicy: directive.mask OR session.masked (run session / sampleSessionId) → ResponseMasker
            └─ samples, schema, run results; step patches mask payload values, keep header names + metadata
```
- `RunRequest.sampleSessionId` (optional): the session inline events were copied from; the Studio sends it with the
  draft's real source connection in its run document (`unbound` only when the draft has none; never read for masking).
---
```
User picks "Decode as Avro" in the sample toolbar
  └─ PreviewActions.reformat → PATCH /api/v1/preview/sessions/{id} {format:"avro"}
       └─ Service.Reformat → owner check (404) → unknown format → 400 /format
            ├─ SessionStore.Reformat (format, TTL slide, cached schema dropped)
            ├─ SampleDecoder.For(avro, connection) → ConnectionRegistry → HTTPRegistryProvider
            │    └─ SpecRegistryConfigs (connection spec from the API, cached 30 s) → registry REST
            ├─ schema inference [async] + RegistrySchemaMerger (writer schema fields, registry=true)
            └─ 200 first page re-decoded (runs keep using the draft's source format)
```
---
```
User clicks "Also write to test topic" in the Test dialog
  └─ POST /api/v1/preview/test-runs {writeToTopic:true}
       ├─ gateway: rate limit → topic.create on branches[0].destination.connectionId? no → 403 /writeToTopic
       └─ Service.Run(test) with OutputCollector (encoded outputs of passed events)
            └─ TestTopicWriter → CreateTopic router.test.<slug>.<ts> (1 partition, 24 h) → produce → registry row
                 └─ 200 RunResult + testTopic → gateway audit preview.test_topic_written
```

## 12. As built (P3)

- `DemoSource` (`internal/preview/source_demo.go`, `SampleMode=demo`, registered in `cmd/preview/assembly.go`): the latest
  `count` (default 50, max 1,000) synthetic `order-events` from connections' `kafka.DemoOrderGenerator` (same seed as the
  demo cluster), 1 s apart ending now, spread over 3 partitions; event i is identical in every process. No Kafka, no
  connection spec read. Topics other than `order-events` → 404 `preview/topic-not-found` at `/topics/{i}`.
- Gateway: `mode=demo` without `connectionId` needs only `route.read`; with a connection it is checked like any load
  (`sample.read` on the connection). Masking still follows `SampleVisibility` for that connection.
- Web: `DemoSamples.applies(connection, topics)` (demo kind + exactly `order-events`) makes `useBuilderPreview` send
  `mode=demo` (ensure-load and the toolbar's Load latest N); time range still reads the demo cluster through Kafka.
  The panel shows "Demo data: synthetic order events (no Kafka read)."
- AI assist masking port (LP-202): `preview.SampleValueMasker` (same `MaskerManager` styles as responses) and
  `gateway.AssistSampleMasker.Mask(ctx, principal, connectionId, samples) → {samples, masked}` (SampleVisibility decides,
  failure = masked); built by `Wiring.previewSampleMasker()` in `cmd/api/wire_preview.go` for transform-studio.
- Sample-access audit actions (LP-203), all filterable in the audit screen (action filter, Access category, object type
  `sample_session`, "Sensitive only" for raw views):

| Action | Category | Sensitive | Object | Written when |
| --- | --- | --- | --- | --- |
| `sample.viewed` | access | no | `sample_session` (`<connection> · <topics>`, `upload`, `demo data`) | masked load |
| `sample.viewed_raw` | access | yes | `sample_session` | unmasked load (any mode) |
| `preview.test_topic_written` | change | no | `topic` | test run wrote a `router.test.*` topic |

  Diff of sample entries: `{connectionId, topics, mode, loaded, masked}` (never payloads).

```
Builder opens a route whose source is the demo sandbox (kind=demo, topic order-events)
  └─ useBuilderPreview → DemoSamples.applies → PreviewActions.ensureLatest({demo:true})
       └─ POST /api/v1/preview/samples {mode:"demo", connectionId, topics:["order-events"], count:50}
            ├─ gateway: sample.read on the connection (none given → route.read) → directive (mask decision)
            └─ preview: SampleSourceManager.For(demo) → DemoSource.Load
                 ├─ topic ≠ order-events → 404 /topics/{i}
                 └─ generate N events within the budget → SessionStore.Create → 201 first page
            └─ audit sample.viewed[_raw] {mode:"demo"} → audit screen label "demo-orders · order-events" / "demo data"
       └─ panel: "50 loaded · …" + demo notice; runs work as for any session
```

Benchmarks (LP-205, `internal/preview/runner_bench_test.go`, recipe route filter + rename + mask + remove on demo
events): `TestPreviewLatencyBudget` asserts 1,000 samples < 2 s and 50-sample re-runs p95 < 300 ms (measured here:
~76 ms per 1,000, ~4–6 ms per 50); `BenchmarkPreviewRun50` / `BenchmarkPreviewRun1000` for profiling.
