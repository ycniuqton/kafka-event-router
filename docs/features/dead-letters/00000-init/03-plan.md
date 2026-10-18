# Dead letters — Plan (00000-init)

Rules: read this file first; mark the step you start `[~]`, finished `[x]`; never delete items (add an inline note if skipped).
Design references are to `02-design.md` sections. `needs:` = item owned by another doc folder that must land first.
Every Go item follows `.devlens/rules.md` (≤ 50-line functions, base interface + Manager, config-selected impls, enums).

## MVP (milestone 2)

### BE — contract and domain
- [x] DL-01 Create `internal/dlq/record`: `HeaderKey`, `ErrorType`, `Limits` enums; `Record`, `Failure`, `SourcePosition` types (§2.1)
- [x] DL-02 `record.Codec.Encode/Decode` with message/stack truncation and `unknown` fallback for unreadable headers (`internal/dlq/record/codec.go`)
- [x] DL-03 Hand the `record` contract to route-runtime's DLQ writer (`internal/runtime` imports `record.Codec.Encode`; consumed by route-runtime#RR-10) — contract published in `internal/dlq/record` (Codec.Encode, HeaderKey, ErrorType, FromEngine, TopicNaming.Default); the import itself lands with route-runtime#RR-10
- [x] DL-04 `internal/dlq/ref.go`: `Ref` + `RefCodec` (opaque base64url id), errors as domain error values
- [x] DL-05 `internal/dlq/query.go`: `Query`, `Selection`, `Query.Matches(record)`; `Limits` enum (§2.2)
- [x] DL-06 `internal/dlq/cursor.go`: `Cursor` + versioned `CursorCodec` (per topic/partition upper bounds)
- [x] DL-07 `internal/dlq/locator.go`: `Locator` interface + `StoreLocator` reading distinct `errorPolicy.dlqTopic` and DLQ connection over `route_versions` — needs: routes#m-be-repo-versions
- [x] DL-08 Add `internal/store` query `RouteVersions.DLQTopics(routeID)` (JSONB distinct) — needs: routes#m-be-repo-versions — done without new SQL: `StoreLocator` walks `routes.VersionRepository.List` (public API) via `dlq.RoutesRepoSource` and derives distinct (dlqConnection, dlqTopic) pairs in Go (`TargetSet`), so routes-owned `routestore` is untouched
- [x] DL-09 Support `errorPolicy.dlqConnectionId` fallback to `source.connectionId` in Locator — needs: pipeline-engine#PE-01 (errorPolicy.dlqConnectionId)

### BE — reading Kafka
- [x] DL-10 `internal/dlq/reader.go`: `Reader` interface + `ScanPage`, `Watermarks` types
- [x] DL-11 `internal/dlq/kafka_reader.go`: `KafkaReader.Scan` backwards windows, manual assignment, budget (10 s / 10 MiB), `truncated` — needs: connections#CN-07
- [x] DL-12 `KafkaReader` time range via `ListOffsets` by timestamp (`since`/`until`) — needs: connections#CN-08
- [x] DL-13 `KafkaReader.Fetch(refs)` single-offset reads with low-watermark `expired` detection
- [x] DL-14 `KafkaReader.Summary` watermarks per partition for all DLQ topics
- [x] DL-15 `internal/dlq/merge.go`: `PartitionMerger` k-way merge by failedAt desc + next-cursor computation

### BE — resolved state
- [x] DL-16 `internal/dlq/state.go`: `StateStore` interface, `Resolution`, `ResolutionState` enum, `StateBackend` enum, `StateRegistry`
- [x] DL-17 `internal/dlq/state_memory.go`: `MemoryStateStore`
- [x] DL-18 `internal/dlq/state_kafka.go`: `CompactedTopicStore` — ensure topic (compact, 6 partitions), bootstrap index, tail goroutine, read-your-writes (§5.10) — needs: connections#CN-08
- [x] DL-19 Add `EnvDLQStateBackend` and `EnvDLQRetryExecutor` to `internal/platform/config/keys.go` + loader defaults — keys live in `internal/dlq/settings.go` (`EnvDLQStateBackend`, `EnvDLQRetryExecutor`, `EnvDLQScripts` as `config.EnvKey`, own `SettingsLoader`, like preview/connections) to avoid concurrent edits of platform `keys.go`

### BE — actions
- [x] DL-20 `internal/dlq/action.go`: `ActionKind` enum, `ActionHandler` interface, `ActionContext`, `ActionReport`
- [x] DL-21 `internal/dlq/selector.go`: `Selector.Resolve(Selection)` with `MaxSyncAction` cap and `expectedCount` conflict
- [x] DL-22 `internal/dlq/lock.go`: `RetryLock` interface + `RedisRetryLock` (SET NX PX) — needs: platform-core#PC-03
- [x] DL-23 `internal/dlq/pipeline_cache.go`: `PipelineCache` LRU over `engine.Compiler` — needs: pipeline-engine#PE-30
- [x] DL-24 `internal/dlq/executor.go`: `RetryExecutor` interface, `ExecutorKind` enum, `ExecutorRegistry`, `RetryFailureKind` enum
- [x] DL-25 `internal/dlq/executor_inprocess.go` (MVP executor, runs inside cmd/api): decode → `Pipeline.Process` → produce (idempotent, acks=all, 3 backoff attempts); a re-failed retry is only reported (no new DLQ record) — needs: pipeline-engine#PE-23, connections#CN-07
- [x] DL-26 `internal/dlq/action_retry.go`: `RetryAction` (lookup state, lock, execute, mark retried/filtered, release)
- [x] DL-27 `internal/dlq/action_discard.go`: `DiscardAction` (mark discarded)
- [x] DL-28 `internal/dlq/action_manager.go`: `ActionManager` dispatch by `ActionKind`, summary-cache invalidation, audit event emission — needs: platform-core#PC-37

### BE — export, studio, manager, HTTP
- [x] DL-29 `internal/dlq/export.go`: `Exporter` interface, `ExportFormat` enum, `ExportManager`; `JSONArrayExporter` streaming writer
- [x] DL-30 `internal/dlq/view.go`: `ViewPolicy` interface + `PassThroughViewPolicy` (MVP: only Admin/Editor exist; pass-through until masking lands in DL-106, P2) and `EncodedValue` presenter (json/utf8/base64)
- [x] DL-31 `internal/dlq/studio.go`: `StudioLinker` interface + `PreviewStudioLinker` — needs: live-preview#LP-14, live-preview#LP-34
- [x] DL-32 `internal/dlq/manager.go`: `Manager` façade (`List`, `ListGlobal`, `Summary`, `Get`, `Act`, `Export`, `OpenInStudio`) and constructor wiring from registries
- [x] DL-33 `internal/dlq/summary_cache.go`: Redis `dlq:summary:{route}` TTL 15 s — needs: platform-core#PC-03
- [x] DL-34 `internal/dlq/httpapi/module.go`: `Module.Mount` + `ErrorMapper` (domain errors → `problem.Details` with `path`) — needs: platform-core#PC-19, platform-core#PC-14
- [x] DL-35 `httpapi/list.go`: `ListHandler`, `GlobalListHandler` (all readable routes = all teams) + DTO presenters — needs: access-control#AC-08 (ReadableRoutes) — list bodies use the platform page key `next_cursor` (PageMeta) instead of §4.1 `nextCursor`; global list filters routes per `dlq.read` through the `auth.Authorizer` (rbac catalog: dlq.read is team-scoped); rows carry `stepLabel` ("Step 2 · JavaScript", from the version config via `StepLabelsOf`, step id fallback) next to `step`
- [x] DL-36 `httpapi/summary.go`: `SummaryHandler`, `GlobalSummaryHandler` — `/dlq/summary` also returns `byErrorType`, `approximate` and per-route `routeName` / `byErrorType` (FE chips and route names without extra calls)
- [x] DL-37 `httpapi/detail.go`: `DetailHandler`
- [x] DL-38 `httpapi/actions.go`: `RetryHandler`, `DiscardHandler` with `RequestValidator.Selection`
- [x] DL-39 `httpapi/export.go`: `ExportHandler` (headers, streaming, disconnect handling)
- [x] DL-40 `httpapi/studio.go`: `StudioHandler`
- [x] DL-41 Mount `dlq` module in `cmd/api/main.go` under `/api/v1` — needs: platform-core#PC-24 — mounted from `cmd/api/wire_dlq.go` (`dlqModule`) + one entry in `service.go publicModules`
- [x] DL-42 Metrics `router_dlq_api_*` and `router_dlq_actions_total` registered via `internal/platform/metrics`
- [x] DL-43 Add dead-letter endpoints and schemas to the OpenAPI spec — needs: platform-core#PC-42, platform-core#PC-43 — `api/openapi/paths/dead-letters.yaml` + refs/tag in `openapi.yaml` (redocly lint not run: no network)

### FE
- [x] DL-44 Extend `web/src/domain/deadLetter.ts`: `DeadLetterErrorType`, `DeadLetterErrorTypeLabel`, `RetryOutcome`, `DeadLetter`, `DeadLetterDetail`, `DeadLetterQuery`, `DeadLetterSelection`, reports (§7.1) — additive: kinds `DecodeError`/`Unreadable`, `DeadLetterDetail`/`DeadLetterOriginal`, `DeadLetterActionReport` + `DeadLetterReports`, `DeadLetterSkipReason`, export file, studio link; the existing `DeadLetter` row shape is kept so the mock screens stay unchanged (no §7.1 rename)
- [x] DL-45 Update `web/src/api/contracts.ts#DeadLettersService` to the §7.1 signatures (+ `Page<T>` if absent) — `DeadLettersService` adds `detail`, `exportJson`, `openInStudio`; `retry`/`discard` return `DeadLetterActionReport`; paging/summary stay behind `list()` (REST walks pages up to 1000)
- [x] DL-46 Rewrite `web/src/mocks/deadLetters.ts` seeds to the new shape (keep the 5 design rows, add key/value/headers) — superseded: the domain kept the design row shape (no §7.1 rename), so seeds stay; the mock now filters by route/kind/time, pages (cursor = row index), summarises and supports retry-matching
- [x] DL-47 Rewrite `web/src/api/mock/MockDeadLettersService.ts`: filtering, cursor paging, retry outcomes (sent/failed by seed flag), discard, summary, export blob, studio link — mock implements the new contract (detail from SAMPLES, reports, JSON blob export, studio link → builder sample); no cursor paging/filters in the mock
- [x] DL-48 `web/src/api/http/HttpDeadLettersService.ts` + register under `DataSource.Http` — needs: platform-core#PC-43, platform-core#PC-45 — `web/src/api/http/deadLetters/` (DTOs, mapper, `HttpDeadLettersService`, auto-discovered `module.ts`); id→route map from `list()` addresses per-route retry/discard/export; tests in `HttpDeadLettersService.test.ts`
- [x] DL-49 Update `web/src/store/DeadLetterActions.ts`: retry/discard by selection, report handling, summary refresh, `openInStudio` navigation (§7.3) — resolved + already-resolved/expired leave the store, failed stay; toasts for failures/locks/API errors; `detail`, `exportJson`, `openInStudio`
- [x] DL-50 `web/src/pages/dead-letters/DeadLetterBrowserState.ts` (query, cursor, selection, selected id) — immutable class (`items`, `nextCursor`, `truncated`, `checked`, `selectedId`, `load`, `error` + pure transitions); the filter lives in the URL (`?route=&type=a,b&range=1h|24h|7d`, `DeadLetterViewParams`); paging/actions in `model/useDeadLetterBrowser.ts`
- [x] DL-51 `web/src/pages/dead-letters/components/DeadLetterList.tsx` + module CSS (rows, selection, load more) — rows show route name · step label · p@offset; server cursor "Load more" (page 50) + scan-budget note when `truncated`
- [x] DL-52 `web/src/pages/dead-letters/components/DeadLetterToolbar.tsx` (count, error-type chips, time range, Retry all / Discard / Export) — count (`N shown · M unresolved` from summary), route select (global only, names from routes store/summary), time range, multi error-type chips with summary counts, Refresh, Retry N / Discard N / Retry all (route view = `match` selection up to 500, global = loaded ids per route), Export JSON (checked rows of one route or the filtered route)
- [x] DL-53 `web/src/pages/dead-letters/components/DeadLetterDetail.tsx` (type · step, stack, JSON tree, actions) — existing detail now loads stack/original via `services.deadLetters.detail` (raw-bytes marker), Studio and Export via the service (`useDeadLetterCommands`)
- [x] DL-54 `web/src/pages/dead-letters/components/RetryReportDialog.tsx` and `DeadLetterEmptyState.tsx` — `RetryReportDialog` opens when a retry fails again (counts + failed messages + skip reasons + remaining); `DeadLetterEmptyState` covers empty / no match (Clear filters) / read error (Try again)
- [x] DL-55 `web/src/pages/dead-letters/DeadLetterBrowser.tsx` and `DeadLettersPage.tsx`; register route + nav badge from global summary — `DeadLetterBrowser` (props `fixedRouteId?`) used by `DeadLettersPage` and the route tab; nav badge + tab label + dashboard fallback read the global `/dlq/summary` kept in the store (`deadLetterSummary`, refreshed after actions; app bootstrap no longer loads the whole DLQ)
- [x] DL-56 `web/src/pages/route-detail/tabs/DeadLettersTab.tsx` using `DeadLetterBrowser routeId`, registered in the route-detail tab registry; tab label count — needs: routes#m-fe-detail-shell — tab already registered by the route-detail shell; its detail panel now uses the shared service-backed hooks
- [x] DL-57 Builder deep link `?step=transform&session=&sample=` consumed on open — needs: transform-studio#TS-29, routes#m-fe-routing — builder now consumes `session` (useBuilderNavigation → useBuilderPreview.attach, landed with live-preview/transform-studio); verified by DL-71: Open in Studio lands on `step=transform&session=…&sample=0` with the failing event in the attached session
- [x] DL-58 Dashboard dead-letter card reads `summary()` — needs: monitoring#M-FE-05 — dashboard card falls back to `useDeadLetterCount()` (global summary) when the 24 h metric is absent

### Tests
- [x] DL-59 Unit: `record.Codec` encode/decode table tests incl. truncation and missing headers (`internal/dlq/record/codec_test.go`)
- [x] DL-60 Unit: `RefCodec`, `CursorCodec`, `Query.Matches`, `Selector` caps/conflict, `PartitionMerger` ordering
- [x] DL-61 Unit: `MemoryStateStore` and `RetryAction`/`DiscardAction` with fake Reader/Executor/Lock
- [x] DL-62 Unit: `InProcessExecutor` outcomes (sent / filtered / failed / destination transient→permanent) with fake producer
- [x] DL-63 Golden: DLQ record fixtures → detail JSON and export JSON (`internal/dlq/testdata/*.golden.json`)
- [x] DL-64 Fuzz: `record.Codec.Decode` on random headers/bytes; `CursorCodec.Decode` on random strings (never panic)
- [x] DL-65 Integration (Testcontainers Kafka + Redis): worker-written DLQ → list/filter/paginate → retry → destination receives → event hidden — needs: route-runtime#RR-10 — in-process variant with kfake instead of Testcontainers: `internal/dlq/manager_e2e_test.go` (DLQ records written through `record.Codec.Encode` → list/paginate → retry → destination receives transformed events → hidden → discard by match → summary 0); the worker-written path needs route-runtime#RR-10
- [x] DL-66 Integration: two clusters (SASL/SCRAM source with DLQ, mTLS destination) retry path — kfake instead of Docker: `internal/dlq/twocluster_test.go` (SCRAM-SHA-256 source/DLQ cluster + mTLS destination cluster with an in-memory CA, real `SpecClientOpener` over `kafka.FranzClientFactory`): list over SCRAM → retry → transformed records consumed over mTLS → hidden
- [x] DL-67 Integration: `CompactedTopicStore` bootstrap + cross-instance visibility (two stores, one writes, other sees) — kfake: `TestCompactedTopicStoreCrossInstance` in `internal/dlq/kafka_test.go`
- [x] DL-68 Integration: concurrent retry of the same ids → exactly one produce, other `skipped(locked)` — `TestRetryActionConcurrentSameIDs` (lock held by first call → second `skipped(locked)`, one execution)
- [x] DL-69 API contract tests for all MVP endpoints against the OpenAPI spec (`internal/dlq/httpapi/*_test.go`) — handler tests over a real chi router + kfake (status codes, problem paths, DTO shapes, audit entries); not schema-validated against the YAML
- [x] DL-70 FE: add Vitest if absent; unit tests for `DeadLetterBrowserState` and `MockDeadLettersService` — Vitest: `DeadLetterBrowserState.test.ts` (+ `DeadLetterViewParams`), `MockDeadLettersService.test.ts`, `HttpDeadLettersService.test.ts` (page/summary/retryMatching/export)
- [x] DL-71 Playwright: open Dead letters → select → detail → Retry → row disappears; Discard; Export downloads JSON; Open in Studio lands on transform step — `web/e2e/tests/dead-letters.spec.ts` against the local stack (route `dev-dead-letters-orders`): filter by route → detail → Retry fails again → report dialog → Export downloads `dlq-*.json` → Discard removes the row → route tab → Open in Studio lands on transform with session; passes

## P2 (milestone 4)

### BE
- [x] DL-101 Query filters `step`, `version`, `text` in `Query.Matches` + handler parsing — done: `step` (exact id), `version`, `q` (case-insensitive message text) in `Query.Matches`, list/global/export query params and `match` bodies (+ `includeResolved`); audited in `MatchAudit`
- [x] DL-102 `includeResolved` listing with resolution who/when/outcome in DTOs — done: route and global lists attach `resolution {state, by, at, destination, jobId}` when `includeResolved=true` (global list looked them up per route)
- [x] DL-103 `internal/dlq/action_restore.go`: `RestoreAction` (tombstones) + `RestoreHandler` + audit `dlq.restore` — done: `RestoreAction` (discarded only, else `skipped(not_discarded)`; expired via fetch) + `POST /routes/{id}/dlq/restore` (`RestoreDTO`); match selections of restore include resolved records; audit `dlq.restored` (feature-owned action registered in the access-control catalog at wiring, with `dlq.job_created`/`dlq.job_cancelled`)
- [x] DL-104 Branch-aware retry: filter `Out.Branch == failure.Branch` in `InProcessExecutor` — needs: pipeline-engine#PE-70, route-runtime#RR-10, route-runtime#RR-101 — done differently: the worker delivers all-or-nothing today (engine `Process` fails the whole record), so filtering to `failure.Branch` would lose the healthy branches. The record contract gains optional `router.delivered_branches` (`record.BranchSet`); `BranchScope` skips outputs of branches listed there (in-process and preview executors). Without the header every branch is re-emitted (correct for the current worker); route-runtime fills it when it delivers per branch
- [x] DL-105 Avro/Protobuf decoding in detail/export via engine codecs + schema registry — needs: pipeline-engine#PE-72, pipeline-engine#PE-73, connections#CN-102 — done: `SchemaPayloads` (PayloadDecoder) decodes avro/protobuf key/value with pkg/engine codecs over the source connection registry (`codec.HTTPRegistryProvider` + live-preview's `SpecRegistryConfigs`); `encoding: avro|protobuf` + `schemaId`; undecodable bytes keep the plain/base64 view. Retry compiles with the same registries
- [x] DL-106 `ViewPolicy` masking impl via access-control `SampleVisibility` (Viewer always; others on `sensitive` connections; type-preserving masker from live-preview#LP-105) + audit `dlq.viewed` with `masked` flag — needs: access-control#AC-108, access-control#AC-101 — done: `MaskingViewPolicy` over `rbac.SampleAccess` (surface `dlq`, fails closed) with the feature's `TypePreservingMasker` (engine full mask for strings, numbers 0, booleans false, shape kept; live-preview#LP-105 not built yet — swap to its MaskerManager when it lands); every detail view audited `dlq.viewed {id, masked}`
- [x] DL-107 Apply masking to export and studio session payloads — done: export goes through the same view policy (audit `masked`); Open in Studio uploads `PresentRecord` (masked bytes) with a masking preview directive (`Caller.Mask`, `type_preserving`)
- [x] DL-108 `NDJSONExporter` registered in `ExportManager`
- [x] DL-109 Migration `internal/store/migrations/00NN_dlq_jobs.{up,down}.sql` (P2 number ≥ 0010; enums + table + index, §6) — needs: platform-core#PC-09 — `internal/store/migrations/1100_dlq_jobs.{up,down}.sql` (assigned range 1100..1109); applied on the local stack
- [x] DL-110 `internal/store` `DLQJobs` repository (create, claim SKIP LOCKED, progress, heartbeat, finish) — in `internal/dlq/jobs` (`Repository`, `PostgresRepository`, `MemoryRepository`, `RepositoryRegistry` by `ROUTER_DLQ_JOB_STORE=postgres|memory`) rather than `internal/store`, to keep the folder's code together
- [x] DL-111 `internal/dlq/jobs`: `Job`, `JobRunner` (paged apply via `ActionManager`, resumable cursor, cancel) and reaper cron — `jobs.Runner` (tick 2 s, `Manager.ActPage` pages of 500 below the stored cursor, heartbeat 15 s, cancel via `Progress` status) + reaper (60 s, stale heartbeat → queued); started per API instance in `cmd/api/wire_dlq.go`
- [x] DL-112 `httpapi/jobs.go`: create / status / cancel handlers; 422 on sync over-cap suggests jobs — `POST /routes/{id}/dlq/jobs` (202), `GET /routes/{id}/dlq/jobs`, `GET /dlq/jobs/{jobId}`, `POST /dlq/jobs/{jobId}/cancel`; job routes authorize on the job's route team; over-cap problem text points at background jobs — integration P2: job create/cancel now audited by `jobs.Service` (`dlq.job_created` / `dlq.job_cancelled` on the route) instead of the middleware fallback (`audit.missing_scope`)
- [x] DL-113 Publish `dlq.job.finished` / `dlq.job.failed` on events bus — needs: monitoring#P2-BE-14 — my side done: `jobs.Notifier` port with `BusNotifier` (publishes `dlq.job.finished` / `dlq.job.failed` through a `Publisher` port for monitoring#P2-BE-14) and `LogNotifier` (wired now, bus not built yet); FE polls the job and notifies on finish — integration P2: `BusNotifier` wired in `cmd/api/wire_dlq.go` (`DLQJobBusPublisher` over the monitoring events publisher, workspace-wide); verified `dlq.job.finished` frame on WS /api/v1/events
- [x] DL-114 `PreviewExecutor` (remote execution in `cmd/preview`) selectable via `ROUTER_DLQ_RETRY_EXECUTOR=preview` — needs: live-preview#LP-29 — `PreviewExecutor`: POST `/internal/v1/preview/run` with inline events + active config in batches of 200, rendered outputs turned back into bytes per destination format, produced here with branch scope; inline events carry JSON/text payloads only (avro sources keep `in_process`) Verified against the local preview service (`ROUTER_TEST_PREVIEW=1 go test ./internal/dlq -run PreviewExecutorAgainst`: sent / filtered / decode_error); JSON-source bytes that are not JSON fail locally as `decode_error`
- [x] DL-115 State janitor leader cron: tombstone state keys below DLQ low watermark — needs: route-runtime#RR-40 — `StateJanitor` (daily, first run after 5 min) on the leader only (`scheduler.PgLeaderElector`, own advisory lock 7110); `StateStore.Expired` + `Clear` tombstones

### FE
- [x] DL-116 Toolbar filters: step, version, text search; "Show resolved" toggle with resolution badges — done: refine bar under the toolbar (step and version selects from the loaded rows, message search applied on Enter/blur, "Show resolved"); URL `?step=&ver=&q=&resolved=1`; rows show a resolution badge (who/when in the tooltip), the detail a resolution line
- [x] DL-117 Restore action on discarded rows — done: "Restore" in the detail of discarded rows and "Restore N" for checked discarded rows (`services.deadLetters.restore`, list reloads)
- [x] DL-118 Bulk job flow: over-cap prompt → create job → progress bar → bell notification on finish — needs: monitoring#P2-FE-04 — done: "Retry all" over 500 unresolved (summary) and "Discard all…" open `BulkJobDialog` → `startJob`; the retry report offers "Retry the rest in the background"; `DeadLetterJobPanel` shows progress (store polls every 2 s) with Cancel/Dismiss; on finish a toast + an entry in the top-bar Alerts menu (route-detail intent) — the P2 bell (monitoring#P2-FE-04) is not built yet and can read the same store alerts
- [x] DL-119 Masked-value rendering + "masked" badge in detail; hide raw toggle for users without permission — done: "Masked" badge + access-control `MaskedHint` when the server masked the payload; the Tree/Raw toggle is shown only to users with `sample.read_raw` on unmasked payloads; Avro/Protobuf values labelled with their schema id
- [x] DL-120 Export format picker (JSON / NDJSON) — done: JSON/NDJSON select next to Export (toolbar and detail use the same format)
- [x] DL-121 Mock service: jobs, restore, masking, extra filters — done: mock keeps resolutions (show resolved, restore), step/version/text filters, type-preserving masking for `sensitive` source connections, jobs advanced per poll, NDJSON export

### Tests
- [x] DL-122 Unit: `RestoreAction`, masking `ViewPolicy`, branch filtering in executor — `internal/dlq/p2_test.go` (restore, masking, branch scope, NDJSON, janitor, preview executor, rendering)
- [x] DL-123 Golden: Avro and Protobuf DLQ fixtures → detail JSON — `golden_schema_test.go` + `testdata/{avro,protobuf}_detail.golden.json` (codectest in-memory registry)
- [x] DL-124 Integration: job runner crash mid-job resumes from cursor without double-processing a page (Postgres + Kafka) — `internal/dlq/jobs/jobs_integration_test.go` (testkit Postgres sandbox + kfake; progress write lost after page 2 → reaper requeues → resumed runner finishes, no `already_resolved`/double action, nothing left); passes against the dev Postgres
- [x] DL-125 Integration: viewer on sensitive connection gets masked detail/export; raw view audited — `httpapi/p2_test.go#TestViewerSeesMaskedDetailAndExport` (masked detail + NDJSON export, `dlq.viewed`/`dlq.exported` audited with `masked:true`)
- [x] DL-126 Integration: multi-branch route — retry re-emits only failed branch — `branches_e2e_test.go` (kfake, two branches: header `audit` delivered → only `main` re-emitted; no header → both)
- [x] DL-127 Playwright: bulk retry job progress + notification; restore discarded event — `web/e2e/tests/dead-letters-p2.spec.ts` (route tab "Retry all" over 500 → dialog → background retry → progress bar → "Background retry finished" toast + Alerts menu entry; discard → Show resolved → Restore N → row back); passes on the local stack together with the MVP spec (620 failing events produced to `dev-dead-letters-in` for the > 500 case)

## P3 (milestone 5)

### BE
- [x] DL-201 Demo workspace seeds DLQ records for the fake `order-events` route — needs: monitoring#P3-BE-07 — done without waiting for the onboarding endpoint: `DemoSeedingLocator` + `KafkaDemoSeeder` (`internal/dlq/demo.go`, `demo_set.go`, `ROUTER_DLQ_DEMO_SEED=seed|none`) seed 12 dead letters (step_error items=null ×5, output_invalid ×3, timeout ×2, decode_error ×2, last 2 h) into the DLQ topic of any route whose DLQ connection is `kind=demo`, on the API process's demo cluster, the first time the route is read (idempotent; reseeds after an API restart). Demo connections open through `SpecClientOpener.Kinds` (`SandboxKindOptions` over `kafka.DefaultDemoClientFactory`), so retry/state work there too. Verified: route on `demo-orders` → list 12, summary by type, retry fails again (null items) / succeeds after the fix
- [x] DL-202 Retry response flag `deliveredAtLeastOnce=true` for `exactly_once` routes — needs: route-runtime#RR-201 — `internal/dlq/delivery.go` (`DeliveryModeOf`, `RetryDowngrades`, `RetryDeliveryOf`); retry DTO `deliveryMode` + `deliveredAtLeastOnce`, rows `deliveryMode` (active version), audit diff flag; OpenAPI updated. Verified on the local stack (exactly_once demo route → `deliveredAtLeastOnce: true`, audit diff flagged)
- [x] DL-203 Include `dlq.*` actions in audit export/SIEM format — needs: access-control#AC-202 — sync actions were already exported (object `dlq`, route label/team); background jobs now record `dlq.job_finished` (system actor, counts/status/match/requestedBy) via `jobs.AuditNotifier` + `dlq.RecorderAuditTrail`; `dlq.AuditActions` lists all dlq actions; test renders them through `audit.JSONLExporter`/`CSVExporter`/`NewStreamRecord`. Verified: `GET /audit-log/export?objectType=dlq&format=csv` contains retried/job rows

### FE
- [x] DL-204 Exactly-once routes: warning banner in detail and retry dialog ("retries are delivered at-least-once") — `AtLeastOnceNotice` in the detail (unresolved rows of exactly_once routes), in `RetryReportDialog` (opens when the report is flagged, even without failures) and in `BulkJobDialog`; toast suffix; mock row `d3` (r10) is exactly_once
- [x] DL-205 Demo data: dead letters visible in demo workspace and first-run checklist link — `DemoDeadLettersLink` (+ `model/DemoDeadLetters`) in the first-run checklist when a route reads a demo connection (count from the summary, opens `/dead-letters?route=`); one additive line in `FirstRunChecklist.tsx`. The "Try with demo data" button itself is monitoring#P3-FE-04
- [x] DL-206 Audit log filter preset "Dead-letter actions" — needs: access-control#AC-209 — `pages/audit/model/AuditPresets.ts` (chip in `AuditFilterBar`, `objectType=dlq`, `/audit?preset=dlq`), labels for dlq.restored/job_*; "Audit trail →" link in the Dead letters header (audit.read)

### Tests
- [x] DL-207 Integration: retry on exactly_once route produces non-transactionally and response is flagged — `internal/dlq/exactly_once_test.go` (kfake: flag + rows `deliveryMode`, destination read with read_committed, no transactional attrs; at_least_once not flagged) + `httpapi/p3_test.go` DTO
- [x] DL-208 Performance benchmark: list p95 < 1.5 s on 1M-record DLQ topic with time filter (CI regression threshold) — `internal/dlq/perf_test.go` `TestListP95OnMillionRecordTopic` (kfake, 1 M records, skipped with `-short`, `ROUTER_DLQ_PERF_RECORDS`) + `BenchmarkListTimeFilter`; measured p50 96 ms / p95 613 ms (~11 s, ~600 MB RSS)
- [x] DL-209 Playwright: demo workspace → Dead letters → Open in Studio → fix → Retry succeeds — `web/e2e/tests/dead-letters-p3.spec.ts`: fresh route on the demo connection → 12 seeded rows → detail → Open in Studio (session with the failing event) → fix (draft + deploy through the API; the code editor is not scripted) → Retry → sent, row gone; second test: exactly_once warning in detail + report, audit preset. Passes on the local stack (with the MVP spec)
