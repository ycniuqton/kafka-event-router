# Route Runtime — Design (00000-init)

Scope: SysDesign §8 (worker runtime) and §9 (scheduler, leases, reconciliation, hot reload, rollout, control-plane-down).
Requirement ids (FR-xx) refer to `01-requirements.md`. Cited folders own everything not described here.

## 1. Placement in the system

```
cmd/api  (N instances)                         cmd/worker (M instances)
 ├─ internal/scheduler   [leader only]          ├─ internal/runtime/reconcile   (desired vs actual)
 │   ├─ LeaderElector (pg advisory lock)        ├─ internal/runtime/runner      (one Runner per leased route)
 │   ├─ AssignmentPlanner + PlacementManager    │   ├─ PartitionLane (per partition, ordered)
 │   ├─ LeaseReaper                             │   ├─ OffsetTracker + CommitScheduler
 │   ├─ RolloutCoordinator                      │   ├─ DeliveryManager → at_least_once | exactly_once
 │   └─ ReplayCoordinator           [P3]        │   ├─ ErrorPolicyManager → dlq | skip | pause
 └─ internal/runtime/controlapi                 │   └─ BackpressureGate
     (/internal/v1 worker endpoints,            ├─ internal/runtime/cpclient   (HTTP to /internal/v1)
      runtime status aggregation)               ├─ internal/runtime/cache      (on-disk config cache)
                                                └─ internal/runtime/producers  (shared ProducerPool)
```

Workers never touch Postgres directly; they talk to the API's internal endpoints (workload identity, owned by
systems/platform-core). Events never pass through the API.

## 2. Go packages and key types

All extensible domains follow: base interface → implementations registered in a `Registry` keyed by an enum →
`Manager` used by business logic; the enum value comes from route config or env config (no type switches).

### 2.1 `internal/runtime/domain` — shared enums and constants

```go
type RunnerState string            // starting, running, draining, paused, error, stopped
type ErrorCategory string          // decode, step, step_timeout, output_invalid,
                                   // destination_transient, destination_permanent, source_transient, source_fatal
type Disposition string            // retry_in_memory, apply_policy, backoff_retry, halt_route
type ReloadKind string             // none, pipeline_swap, full_restart
type RecordOutcome string          // produced, filtered, dropped, dead_lettered, skipped
type HaltReason string             // error_policy_pause, destination_permanent, source_fatal, auto_rollback_failed

type Timings struct{}              // domain constant holder (methods return durations):
//   CommitInterval 1s, BackoffMin 100ms, BackoffMax 30s, BackoffFactor 2,
//   LeaseTTL 30s, HeartbeatInterval 10s, ResyncInterval 15s, AutoRollbackWindow 60s,
//   DrainTimeout 30s, StopGracePeriod 30s
type DLQHeader string              // router.route_id, router.version, router.step, router.error_type,
                                   // router.error_message, router.source_topic, router.source_partition,
                                   // router.source_offset, router.failed_at
type TopicNaming struct{}          // DLQDefault(routeName) = "router.dlq.<route>"
```

`RunnerState` maps to `route_observed_state` via `RunnerState.Observed()` (starting/running/draining → running,
paused/stopped → paused, error → error).

### 2.2 `internal/runtime/runner` — processing loop (§8.1)

```go
// Runner runs one route version on this worker; owned by RunnerManager.
type Runner interface {
    RouteID() string
    Version() VersionRef                       // {VersionID, VersionNo, ConfigHash}
    State() RunnerState
    Start(ctx context.Context) error
    Swap(ctx context.Context, next CompiledVersion) error   // pipeline_swap path
    Stop(ctx context.Context, reason StopReason) error      // drain + commit + leave group
    Status() ReplicaStatus
}

type RunnerManager interface {          // only entry point for reconcile
    Ensure(ctx context.Context, lease Lease, v CompiledVersion) error
    Remove(ctx context.Context, routeID string, reason StopReason) error
    Snapshot() []ReplicaStatus
}

type CompiledVersion struct {
    Ref      VersionRef
    Config   config.Route                  // typed doc from pkg/config (systems/pipeline-engine)
    Pipeline engine.Pipeline
    Codecs   engine.CodecManager
}
```

Runner internals (classes, one responsibility each):

| Class | Responsibility |
| --- | --- |
| `SourceConsumer` | Wraps franz-go client from `pkg/kafka` factory; poll, pause/resume partitions, revoke/assign callbacks, `ResetOffset` from `startFrom` |
| `PartitionLane` | One goroutine per assigned partition; processes records in order, hands outputs to `DeliveryStrategy` |
| `InFlightLimiter` | Weighted semaphore of `limits.maxInFlight` records shared by lanes of one runner |
| `RecordProcessor` | decode → `Pipeline.Process` → encode per branch; returns `[]Outgoing` or a classified error |
| `OffsetTracker` | Per partition: ordered pending set; `MarkDone(offset)` advances the contiguous watermark |
| `CommitScheduler` | 1 s ticker + revoke/swap/stop hooks; commits watermarks via `DeliveryStrategy.Commit` |
| `BackpressureGate` | Pauses fetch on a partition when its lane is blocked (limiter full, producer backoff) |
| `RetryPolicy` | Exponential backoff 100 ms → 30 s (jittered), unbounded for transient destination errors |
| `RunnerMetrics` | Emits §12 metrics via the catalogue from features/monitoring |
| `TailTap` | Runner hook calling monitoring's `Tap.Offer`; flag check, 20/s rate limit and Redis sink are monitoring's `RedisTap` [P3] |

`OffsetTracker` detail: each record gets a `pending` counter = number of outputs (0 for filtered). Producer ack
callbacks decrement; at 0 the offset is done. Watermark = first not-done offset − 1; commit offset = watermark + 1.

P2 additions:
- Multi-branch (FR-07): `Outgoing.Branch` carries the producing branch; the record's `pending` counter spans every
  output of every branch, so the offset is done only when all branches acked. Metric labels (`RouteBranches`):
  record-level series (consumed, filtered = no output on any branch, latency, lag, runner state, failures without
  a branch) carry the primary (first) branch, so route sums count each record once; `produced` carries the branch
  of each output and DLQ/skip of a branch failure (`output_invalid`) carry that branch.
- `LagProbe` (every `Timings.LagProbe` = 10 s) reads the end offsets and group commits of this member's partitions
  with kadm on the consumer's own client. It raises the partition watermarks and gives the lag before the first
  local commit, so `router_consumer_lag` keeps growing while fetching is paused (destination outage, backpressure).
  Revoked partitions set their lag series to 0 so `sum(router_consumer_lag)` never counts a partition twice.
- `Stop` bounds the producer flush by `Timings.Drain`: unflushed records are uncommitted and read again.

### 2.3 `internal/runtime/delivery` — delivery strategies (§8.1, §8.2)

```go
type DeliveryStrategy interface {        // base; one per config.DeliveryMode
    Mode() config.DeliveryMode
    Produce(ctx context.Context, out Outgoing, onAck func(error))
    Commit(ctx context.Context, marks []PartitionMark) error
    BeginBatch(ctx context.Context) error   // no-op for at_least_once
    EndBatch(ctx context.Context, commit bool) error
    Close(ctx context.Context) error
}
type DeliveryManager interface { For(v CompiledVersion, deps DeliveryDeps) (DeliveryStrategy, error) }
```

- `AtLeastOnceStrategy` [MVP]: idempotent producers from `ProducerPool` (acks=all), offsets committed with
  `CommitOffsetsSync` after acks.
- `ExactlyOnceStrategy` [P3]: franz-go `GroupTransactSession`; one transaction per polled batch containing outputs,
  DLQ writes and `SendOffsetsToTransaction`; on abort the session rewinds to last committed offsets. Only selectable
  when source and every destination share one connection (validated by systems/pipeline-engine; re-checked at start).

### 2.4 `internal/runtime/producers` — `ProducerPool` (FR-08)

Key = `connectionID + credentialHash + deliveryMode`. Ref-counted; routes on the same destination connection with
the same credentials share one idempotent producer (P2). Transactional producers are never shared (one per runner).
MVP: pool exists but each runner gets its own lease on a pooled client (sharing toggled by `ProducerSharing` enum).
P2: `ROUTER_WORKER_PRODUCER_SHARING` defaults to `on` (unknown values fail worker start). Shared clients lift
franz-go's 10k buffered-record cap (`SharedBufferedRecords`): each runner already bounds its records by
`limits.maxInFlight`, so one slow route cannot block produces of the others. `router_worker_producer_clients`
shows the open clients.

### 2.5 `internal/runtime/errors` — classification and policies (§8.3)

```go
type ErrorClassifier interface { Classify(err error) ErrorCategory }       // typed errors from engine/sandbox/kafka
type DispositionTable interface { For(cat ErrorCategory) Disposition }     // fixed domain table below
type ErrorHandler interface {                                              // base; one per config.ErrorAction
    Action() config.ErrorAction
    Handle(ctx context.Context, f Failure) (RecordOutcome, error)          // error => halt route
}
type ErrorPolicyManager interface { Resolve(policy config.ErrorPolicy) ErrorHandler }
type DLQWriter interface { Write(ctx context.Context, f Failure) error }   // builds headers, produces
```

| ErrorCategory | Disposition | Then |
| --- | --- | --- |
| decode | apply_policy | DLQ raw bytes / skip / pause |
| step | apply_policy | as above |
| step_timeout | retry_in_memory (≤ maxRetries) | then apply_policy |
| output_invalid [P2] | apply_policy | as above |
| destination_transient | backoff_retry | pause partition fetch, no commit past record |
| source_transient | backoff_retry | client-level retry, runner stays running |
| destination_permanent | halt_route | Stop, report error, halt |
| source_fatal | halt_route | Stop, report error, halt |

`output_invalid` [P2]: encode-time failures of destination codecs (Avro/Protobuf schema mismatch, size limit) come from
the engine as `ErrOutputInvalid` and follow the policy (DLQ header `router.error_type=output_invalid`, branch set).
`EngineErrors` refines it: an `output_invalid` whose cause is a timeout, a `net.Error` or an error with
`Temporary() bool == true` (schema registry outage) is `destination_transient`: backoff and reprocess, never DLQ.

Handlers: `DLQHandler` (default), `SkipHandler` (drop + `router_events_dlq_total{error_type}` style counter
`skipped`), `PauseHandler` (returns `ErrHaltRoute{HaltReason: error_policy_pause}`).

`DLQWriter` target: `DLQTargetResolver` returns `errorPolicy.dlqConnectionId` (added to the route schema by
pipeline-engine) or, when absent, the source connection. Topic created on first runner start through `pkg/kafka`
admin ops if missing (partitions = source partition count, cleanup = delete, retention 7 days).

### 2.6 `internal/runtime/reconcile` — worker reconciliation (§9.2, §9.5)

```go
type Reconciler struct { cp ControlPlane; cache ConfigCache; runners RunnerManager; compiler VersionCompiler }
type ControlPlane interface {                       // cpclient implements over HTTP
    Heartbeat(ctx context.Context, hb Heartbeat) (HeartbeatReply, error)
    Assignments(ctx context.Context, workerID string) (AssignmentSet, error)
    VersionConfig(ctx context.Context, ref VersionRef) ([]byte, error)
    ReportStatus(ctx context.Context, s []ReplicaStatus) error
    ReportHalt(ctx context.Context, h HaltReport) error
    Changes(ctx context.Context) (<-chan ChangeHint, error)   // SSE stream relayed from pg NOTIFY
}
type ConfigCache interface {                        // FileConfigCache under ROUTER_WORKER_CACHE_DIR
    SaveAssignments(AssignmentSet) error; LoadAssignments() (AssignmentSet, error)
    SaveVersion(ref VersionRef, doc []byte) error; LoadVersion(ref VersionRef) ([]byte, error)
}
type VersionCompiler interface { Compile(ctx context.Context, ref VersionRef, doc []byte) (CompiledVersion, error) }
type ReloadClassifier interface { Classify(prev, next config.Route) ReloadKind }
```

Secrets are fetched per connection via the internal secrets endpoint (systems/platform-core) at runner start and kept
in memory only; never cached on disk.

### 2.7 `internal/scheduler` — leader-only, inside `cmd/api` (§9.1, §9.4)

```go
type LeaderElector interface { Campaign(ctx context.Context) (lost <-chan struct{}, err error) } // pg_try_advisory_lock
type PlacementStrategy interface {          // base; selected by ROUTER_SCHEDULER_PLACEMENT
    Kind() PlacementKind                    // least_loaded (MVP); others later (e.g. pinned_zone)
    Place(route RouteDemand, pool []WorkerLoad, current []Assignment) []PlacementDecision
}
type PlacementManager interface { Place(...) []PlacementDecision }
type AssignmentPlanner struct{ ... }       // computes desired assignments for all schedulable routes
type LeaseReaper struct{ ... }             // removes expired leases, but see 9.5 semantics
type RolloutCoordinator struct{ ... }      // advances target_version_id one replica at a time; auto-rollback
type ReplayCoordinator struct{ ... }       // [P3] stop → empty group → reset offsets → resume
type SchedulerLoop struct{ ... }           // ticker (5 s) + ChangeHint driven; runs the above in order
```

`LeaderGate` [P2] (`IsLeader()`, implemented by `LeaderState` that `SchedulerLoop` sets while it leads) lets other
features' leader-only jobs (monitoring's `AlertEvaluatorJob`) run on the scheduler leader only; cmd/api hands it out
with `apiService.runtimeLeaderGate()`.

Load score of a worker = Σ route weights (weight = 1 + source partitions assigned / 10, domain constant class
`LoadWeights`) ÷ capacity. Workers report `capacity` and `refused` routes in heartbeats.

### 2.8 `internal/runtime/controlapi` — internal worker API (mounted in `cmd/api`)

Mounted under `/internal/v1`, workload identity required (platform-core middleware). Handlers are classes
(`HeartbeatHandler`, `AssignmentsHandler`, `VersionConfigHandler`, `StatusHandler`, `HaltHandler`, `ChangesHandler`).
`RuntimeStatusAggregator` folds per-replica statuses into `route_runtime` and emits runtime events (route state
changed, route halted, deployment active/failed) onto the event bus consumed by features/monitoring (`WS /events`).

## 3. Data model touchpoints

Uses existing tables from `0001_init`: `routes` (desired_state, active_version_id, replicas), `route_versions`
(config, config_hash), `deployments` (status), `route_runtime`, `workers`, `route_assignments`.

Migration `0008_route_runtime` (fixed MVP number; includes the NOTIFY trigger). `deployments.previous_version_id`
and `failure_reason` are owned by routes (`0007_routes_lifecycle`); this folder only adds `activated_at`:

| Change | Why |
| --- | --- |
| `workers` + `zone text`, `capacity_used double precision NOT NULL DEFAULT 0`, `draining boolean NOT NULL DEFAULT false`, `started_at timestamptz` | Placement spread and load score, graceful drain |
| `route_assignments` + `target_version_id uuid REFERENCES route_versions`, `reported_version_id uuid`, `runner_state route_observed_state`, `error_message text`, `lag bigint`, `reported_at timestamptz` | One-at-a-time rollout and per-replica status |
| `route_runtime` + `running_version_id uuid`, `halted_at timestamptz`, `halt_reason text`, `error_category text` | Halt without restart loop; UI shows running version |
| `deployments` + `activated_at timestamptz` | Auto-rollback window (60 s) |
| trigger `router_runtime_notify` on `routes`, `route_assignments`, `deployments` → `pg_notify('router_runtime', json{route_id,kind})` | Fast reconciliation |

P3 migration `2060_route_replays` (route-runtime's P3 range 2060..2069): `replay_status` enum
(pending/stopping/resetting/done/failed) + `route_replays` (id, route_id, from_ts, status, requested_by,
requested_at, started_at, finished_at, error, consumer_group, partitions) for replay coordination and audit;
partial unique index = one open replay per route; trigger `route_replays_runtime_notify` → `router_runtime`
NOTIFY with `kind=replay` (scheduler wakes up, monitoring's `WS /events` relay sees it).

## 4. API owned

### 4.1 Internal (worker ↔ API), `/internal/v1`

| Method + path | Request | Response |
| --- | --- | --- |
| `POST /workers/{id}/heartbeat` | `{host, zone, version, capacity, capacityUsed, refusedRouteIds[]}` | `{leaseTtlMs, heartbeatMs, assignmentsEtag}` |
| `GET /workers/{id}/assignments` | `If-None-Match: etag` | `{etag, assignments:[{routeId, routeName, versionId, versionNo, configHash, leaseExpiresAt}]}` |
| `GET /route-versions/{versionId}/config` | — | route config JSON (immutable, cacheable) |
| `POST /workers/{id}/status` | `{replicas:[{routeId, versionId, state, errorCategory?, errorMessage?, lag, cpuMs, memBytes}]}` | `204` |
| `POST /routes/{id}/halt` | `{workerId, versionId, reason: HaltReason, errorCategory, message}` | `204` |
| `GET /changes` (SSE) | — | stream of `{routeId, kind}` hints |

### 4.2 Public (`/api/v1`, RFC 7807, audited)

| Method + path | Request | Response | Phase |
| --- | --- | --- | --- |
| `GET /routes/{id}/runtime` | — | `{observedState, runningVersionNo, haltedAt?, haltReason?, errorMessage?, replicas:[{workerId, state, versionNo, lag, reportedAt}]}` | MVP |
| `GET /workers` (admin) | cursor page | `[{id, host, zone, version, capacity, capacityUsed, lastHeartbeatAt, routes}]` | MVP |

Replay endpoints (`POST /routes/{id}/replay`, `GET /routes/{id}/replays/{replayId}`) are owned by features/routes;
this folder implements their `OffsetResetter` port (`ReplayService` over `ReplayCoordinator`, P3).
Pause/resume/deploy/rollback endpoints are owned by features/routes; they only change `desired_state`,
`active_version_id` and `deployments`, which this system reacts to. Resume and a new deploy clear `halted_at`.

## 5. Flows

### 5.1 Record processing (at-least-once)

```
[event] SourceConsumer.Poll returns batch
└─ split by partition → PartitionLane.Enqueue (in order)
   └─ InFlightLimiter.Acquire
      ├─ limiter full → BackpressureGate.Pause(partition) → resume when slots free
      └─ slot acquired → OffsetTracker.Track(offset)
         └─ RecordProcessor.Process (decode → Pipeline.Process → encode per branch)
            ├─ zero outputs → OffsetTracker.MarkDone → outcome filtered/dropped, metric router_events_filtered_total++
            ├─ N outputs → DeliveryStrategy.Produce ×N [async]
            │  └─ [async] producer ack
            │     ├─ all N acked → OffsetTracker.MarkDone → router_events_produced_total += N, latency observed
            │     └─ ack error → ErrorClassifier → see 5.2
            └─ processing error → ErrorClassifier → see 5.2
---
[cron] CommitScheduler tick (1 s)
└─ OffsetTracker.Watermarks() per partition
   ├─ watermark advanced → DeliveryStrategy.Commit → consumer group offsets updated
   └─ unchanged → no-op
---
[event] partitions revoked (rebalance)
└─ PartitionLane.Drain (wait in-flight up to DrainTimeout)
   └─ CommitScheduler.CommitNow → offsets committed for revoked partitions
      └─ lanes closed → OffsetTracker.Forget(partitions)
```

### 5.2 Error disposition

```
[event] RecordProcessor or producer ack returns error
└─ ErrorClassifier.Classify → ErrorCategory
   └─ DispositionTable.For(category)
      ├─ retry_in_memory (step_timeout) and attempts < maxRetries → reprocess record, metric router_js_timeouts_total++
      ├─ retry_in_memory exhausted → apply_policy branch below
      ├─ apply_policy → ErrorPolicyManager.Resolve(errorPolicy).Handle
      │  ├─ dlq → DLQWriter.Write [async ack] → MarkDone, router_events_dlq_total{error_type}++
      │  │  └─ DLQ produce fails → treated as destination_transient (no MarkDone)
      │  ├─ skip → MarkDone, skipped counter++
      │  └─ pause → ErrHaltRoute(error_policy_pause) → halt flow 5.3
      ├─ backoff_retry → BackpressureGate.Pause(partition) → RetryPolicy.Wait → re-produce same record (no commit past it)
      └─ halt_route → halt flow 5.3
```

### 5.3 Route halt

```
[event] ErrHaltRoute raised inside Runner
└─ Runner.Stop(reason=halt) → drain in-flight, commit done offsets, leave group
   └─ ControlPlane.ReportHalt [async, retried]
      ├─ halted version is rolling out → deployment failed → routes auto_rollback version (5.6)
      ├─ halted version is no longer the route's active version (late replica of a rolled-back version) → dropped
      ├─ API reachable → route_runtime: observed=error, halted_at, halt_reason, error_message → runtime event "route.halted" (alerts, WS /events)
      │  └─ SchedulerLoop sees halted route → delete its route_assignments → other replicas stop on next reconcile
      └─ API unreachable → keep halt locally (runner stays stopped), resend on reconnect
---
[http] POST /api/v1/routes/{id}/resume or new deploy (features/routes)
└─ clear route_runtime.halted_at → pg_notify(router_runtime)
   └─ SchedulerLoop → AssignmentPlanner re-creates leases → workers start runners
```

### 5.4 Worker reconciliation

```
[cron] Reconciler tick (15 s) or [event] ChangeHint from GET /internal/v1/changes
└─ ControlPlane.Assignments(workerID)
   ├─ success → ConfigCache.SaveAssignments
   │  └─ diff(desired, RunnerManager.Snapshot)
   │     ├─ lease present, no runner → load config (cache → VersionConfig) → VersionCompiler.Compile → RunnerManager.Ensure → runner starting
   │     │  └─ compile fails → ReportStatus(state=error, category=compile) → RolloutCoordinator marks deployment failed
   │     ├─ runner present, lease absent → RunnerManager.Remove(reason=unassigned) → drain, commit, leave group
   │     ├─ runner version ≠ target version → hot reload flow 5.5
   │     └─ equal → no-op
   └─ failure (timeout, 5xx, network) → control-plane-down flow 5.7
└─ ControlPlane.ReportStatus(RunnerManager.Snapshot) → route_assignments reported_* columns
```

### 5.5 Hot reload of a new version on one replica

```
[event] Reconciler finds target_version_id ≠ runner version
└─ VersionCompiler.Compile(next) in background (old runner keeps processing)
   ├─ compile fails → keep old version, ReportStatus(error, compile) → deployment failed (5.6)
   └─ compiled → ReloadClassifier.Classify(prev, next)
      ├─ pipeline_swap (filter/transform/limits/format-only) → Runner.Swap
      │  └─ stop fetch → drain in-flight → commit → swap Pipeline + codecs → resume fetch (same group member, no rebalance)
      │     └─ ReportStatus(version=next, state=running)
      ├─ full_restart (source conn/topics/pattern/group, destination conn, delivery mode) → Runner.Stop → new Runner.Start
      │  └─ rebalance happens → ReportStatus(version=next)
      └─ none (same config_hash) → ReportStatus(version=next)
```

### 5.6 Rollout and auto-rollback (scheduler side)

```
[event] pg_notify router_runtime kind=deployment (deploy tx by features/routes set active_version_id, deployment rolling_out)
└─ RolloutCoordinator.Advance(route)
   └─ pick one assignment with target_version_id ≠ active → set target_version_id = active
      └─ [async] worker reconciles (5.5) and reports
         ├─ reported_version = target and state running → next replica (repeat Advance)
         │  └─ all replicas on new version → deployment active, activated_at=now, previous deployment superseded → event "deployment.active"
         └─ reported state error or compile failure → deployment failed, failure_reason → auto-rollback below
---
[cron] RolloutCoordinator tick (5 s) within AutoRollbackWindow (60 s) after activated_at
└─ any replica of the new version reports error
   ├─ previous_version_id present → routes DeploymentLifecycle.MarkFailed(deployment, reason) (in-process)
   │  └─ one tx: deployment failed, new version v(max+1) (origin auto_rollback, config = previous),
   │     new deployment(rolling_out), routes.active_version_id = new, audit entry → event "deployment.rolled_back"
   └─ no previous version → deployment failed, route halted (auto_rollback_failed)
```

### 5.7 Control plane unreachable

```
[event] ControlPlane call fails (heartbeat or assignments)
└─ Reconciler enters Degraded mode (metric router_worker_control_plane_up=0)
   ├─ runners keep running on ConfigCache assignments, even after lease_expires_at
   ├─ new runner needed and version in cache but secret not in memory → cannot start → logged, retried
   └─ [cron] retry with RetryPolicy until reachable
      └─ reachable → full resync (5.4); routes absent from the explicit answer are stopped
---
[cron] LeaseReaper tick (leader) — worker heartbeat older than LeaseTTL
└─ delete that worker's route_assignments → AssignmentPlanner places replacements
   └─ old worker (if alive but partitioned) keeps running until it gets an explicit answer; duplicates are harmless (same consumer group)
```

### 5.8 Scheduler leadership and planning

```
[event] cmd/api start
└─ LeaderElector.Campaign (pg_try_advisory_lock(SchedulerLockKey))
   ├─ acquired → SchedulerLoop.Run: [cron] every 5 s + on ChangeHint
   │  └─ LeaseReaper → AssignmentPlanner → PlacementManager.Place → RolloutCoordinator → ReplayCoordinator [P3]
   │     ├─ route running, not halted, has version, under-replicated → insert assignments (lease = now + 30 s, target = active)
   │     ├─ over-replicated or worker draining → delete surplus assignments (highest load first)
   │     └─ route paused/archived/halted → delete assignments → observed paused once replicas report stopped
   ├─ not acquired → standby, retry every 5 s
   └─ [event] DB connection lost → lock lost → stop loop immediately → standby
---
[http] POST /internal/v1/workers/{id}/heartbeat
└─ upsert workers (host, zone, version, capacity, capacity_used, last_heartbeat_at)
   └─ extend lease_expires_at = now + LeaseTTL for that worker's assignments
      └─ refusedRouteIds non-empty → delete those assignments → replanned elsewhere
```

### 5.9 Replay [P3]

```
[http] POST /api/v1/routes/{id}/replay {from}   (handler + RBAC + confirm owned by features/routes)
└─ routes ReplayManager → OffsetResetter.Reset (ReplayService here) → insert route_replays(pending) → 202 from routes
   └─ [cron] ReplayCoordinator (leader)
      └─ status stopping → assignments suspended (route treated as paused) → wait until replicas report stopped
         └─ kadm DescribeGroups(group)
            ├─ group empty → status resetting → ListOffsets(from) → CommitOffsets(group) → status done → assignments restored → event "route.replayed"
            ├─ group not empty after 60 s → status failed("group still has members") → assignments restored
            └─ ListOffsets error → status failed(error) → assignments restored
```

### 5.10 Graceful worker shutdown

```
[event] SIGTERM
└─ ControlPlane.Heartbeat(draining=true) → scheduler moves leases away
   └─ RunnerManager.StopAll(StopGracePeriod) → each runner drains, commits, leaves group
      └─ process exit (Kafka rebalance hands partitions to other replicas)
```

### 5.11 Multi-branch record and lag while paused [P2]

```
[event] PartitionLane hands record R to RecordFlow
└─ RecordProcessor.Process → Pipeline.Process → outputs tagged by branch → Encode per branch destination
   ├─ encode fails (schema mismatch) → output_invalid → errorPolicy (dlq: header branch + error_type) → MarkDone
   ├─ encode fails (registry timeout / temporary) → destination_transient → backoff → reprocess R (no DLQ, no commit)
   ├─ no output on any branch → MarkDone, router_events_filtered_total{branch=primary}++
   └─ outputs on branches a, b → pending = |outputs| → Produce each on its branch connection [async]
      ├─ all acked → MarkDone(R), router_events_produced_total{branch=a|b} += per-branch count
      └─ one branch transient → that output re-produced after backoff; R stays pending (no commit past R)
---
[cron] LagProbe tick (10 s, per runner)
└─ kadm Lag(group) on the consumer client → keep partitions owned by this member
   ├─ end offset > fetched watermark (fetch paused) → watermark raised → lag = end − local commit
   ├─ no local commit yet → lag = probed group lag
   └─ router_consumer_lag{partition} set → monitoring lag_above reads sum by route
---
[event] partitions revoked
└─ forget(tps) → router_consumer_lag{partition} = 0 on this worker (new owner reports it)
```

### 5.12 Exactly-once batch [P3]

```
[event] poll returns a batch (delivery.mode = exactly_once; source = GroupTransactSession, read_committed)
└─ PollCycles.For(strategy.Batching()=transactional) → TransactionalCycle
   ├─ no records → dispatch high watermarks only (no transaction)
   └─ BeginBatch (session.Begin) → BatchBarrier.Expect(n) → dispatch to lanes (per-partition order kept)
      └─ each record → outputs + DLQ writes produced inside the transaction [async acks]
         ├─ every record finished → EndBatch(commit) = flush + TxnOffsetCommit(polled offsets) + EndTxn
         │  ├─ committed → AllowRebalance → next poll
         │  └─ rebalance / commit refused → aborted → session rewinds to last commit → tracker forgotten → batch read again
         ├─ record abandoned (halt) / produce failed / partitions lost / stop / 30 s batch timeout
         │  → batch ctx cancelled, lanes drained, fetch pauses lifted → EndBatch(abort) → rewind → read again
         └─ Begin/End error (txn state failed) → HaltError{transaction_failed} (Recoverable)
            → Manager stops the runner, waits RunnerRecovery 2 s, starts it again (no route halt, no restart loop on the route)
---
[event] runner start with exactly_once
└─ SameConnectionGuard: every resolved connection (destinations, DLQ) == source → else halt delivery_mode_forbidden
   └─ TransactSourceMode opens the session: transactional id router.<route>.<uuid>, RequireStableFetchOffsets
```

Outputs of an aborted batch stay in the log as aborted records (invisible to `read_committed` readers);
outcome metrics count them when the record finished, so an abort can over-count `produced` by one batch.

### 5.13 Replay (implemented) [P3]

```
[http] routes POST /routes/{id}/replay → routes' OffsetResetter → ReplayService.Reset(routeID, from, actor)
└─ route exists? active version? → insert route_replays(pending) (second open replay → ErrReplayInProgress / 409)
   └─ [event] NOTIFY kind=replay → [cron] SchedulerLoop.Pass (leader): Reaper → ReplayCoordinator → Planner → Rollout
      ├─ pending → stopping (started_at) → RouteDemand.Replaying → planner removes the route's leases
      │  └─ workers stop runners (drain, commit, leave group)
      ├─ stopping: leases gone and DescribeGroup(group) empty → resetting (consumer_group recorded)
      │  └─ still members after Timings.ReplayWait (60 s) → failed("consumer group … still has N members")
      ├─ resetting: OffsetsForTimes(from, source topics | topicPattern matches) → CommitOffsets(group)
      │  ├─ ok → done (partitions) → planner gives the leases back → runners re-read from `from`
      │  └─ error → failed(error) → leases back, offsets untouched
      └─ GET /routes/{id}/replays/{replayId} → ReplayService.Status
```

### 5.14 Live tail call site [P3]

```
[event] record finished (produced | filtered | dead_lettered | skipped)
└─ TailHook.Observe → TailTap.Active(routeID)   (one cached flag check; NoopTail when nothing is wired)
   ├─ false → return (0 allocations, ~12 ns)
   └─ true → TailTap.Offer(routeID, TailEvent{partition, offset, branch, outcome, step, error, input, outputs})
      └─ monitoring's RedisTap (adapter in cmd/worker/tail.go): 20/s token bucket, XADD
```

## 6. Configuration (env, extends `internal/platform/config`)

| EnvKey | Default | Used by |
| --- | --- | --- |
| `ROUTER_WORKER_ID` | hostname | worker identity (stable pod name in k8s) |
| `ROUTER_WORKER_ZONE` | `default` | placement spread |
| `ROUTER_WORKER_CAPACITY` | `100` | load score |
| `ROUTER_WORKER_CACHE_DIR` | `/var/lib/router/cache` | `FileConfigCache` |
| `ROUTER_SCHEDULER_PLACEMENT` | `least_loaded` | `PlacementManager` |
| `ROUTER_SCHEDULER_ENABLED` | `true` | API can opt out of campaigning |
| `ROUTER_DLQ_DEFAULT_TARGET` | `source` | `DLQTargetResolver` (`source` / `destination`) |
| `ROUTER_WORKER_PRODUCER_SHARING` | `on` (P2; `off` in MVP) | `ProducerPool` (`on` / `off`) |

## 7. Metrics emitted (names owned by features/monitoring)

`router_events_consumed_total`, `router_events_filtered_total`, `router_events_produced_total`,
`router_events_dlq_total{error_type}`, `router_consumer_lag{partition}`, `router_end_to_end_latency_seconds`,
`router_step_duration_seconds{step}` (from engine trace-lite timers), `router_js_timeouts_total`,
`router_js_memory_bytes`, `router_runner_state`; plus runtime-only: `router_worker_control_plane_up`,
`router_runner_reloads_total{kind}`, `router_scheduler_leader`, `router_worker_capacity_used`,
`router_worker_producer_clients` (P2).

## 8. Error cases

| Case | Behaviour |
| --- | --- |
| Compile failure of new version | old version keeps running; deployment failed |
| Rebalance during swap | swap aborted, revoke path commits, runner resumes on new assignment with next pipeline |
| Commit fails (group generation changed) | logged, watermarks kept; records reprocessed by new owner (at-least-once) |
| Secret fetch fails at start | runner `error`, retried with backoff; not a halt |
| DLQ topic cannot be created (ACL) | halt route `destination_permanent` with message naming the DLQ topic |
| Partition count < replicas | replicas capped to partitions (FR-21) |
| Worker over capacity | refuses in heartbeat; scheduler replans |
| Two scheduler leaders (split brain) | prevented by advisory lock tied to DB session; writes guarded by `pg_try_advisory_xact_lock` check |
| Exactly-once with cross-cluster config | runner refuses to start, `source_fatal`-equivalent halt with validation message |

## 9. Testing (SysDesign §13)

Unit: OffsetTracker (property tests with random ack orders), DispositionTable, ReloadClassifier, placement,
RetryPolicy. Integration (Testcontainers Kafka/Postgres): end-to-end route, DLQ headers, halt, hot reload without
rebalance, rollout + auto-rollback, replay, exactly-once. Chaos: worker kill, broker restart, network partition,
control-plane down — assert no loss, measure duplicates. Benchmarks: events/sec per core, p95 latency.

## 10. Decisions

1. Auto-rollback creates a new version that copies the previous one, the same as a manual rollback. This folder
   detects the failure and calls routes' `DeploymentLifecycle.MarkFailed`. routes creates the version
   (`origin=auto_rollback`).
2. Replay: routes owns `POST /routes/{id}/replay` (+ status GET) and monitoring owns the confirm dialog in the
   route-detail header. This folder owns the mechanism (`ReplayCoordinator`, `ReplayService` implementing
   routes' `OffsetResetter`, `route_replays` table, P3).
3. The DLQ target is `errorPolicy.dlqConnectionId` (added to the schema by pipeline-engine) and defaults to the source
   connection. DLQ topics have 7-day retention.
4. Paused routes stop their runners (leave the group).
5. Migration `0008_route_runtime` includes the `router_runtime_notify` trigger. `deployments.previous_version_id`
   and `failure_reason` come from routes `0007`.
6. Fresh JS context per call and `ctx.now` frozen per event (processing time) are inherited from js-sandbox/pipeline-engine.
7. A halt report for a version that is no longer the route's active version is ignored, so the second replica of a
   rolled-back version cannot pause the route that auto-rollback just restored.
8. Saving a draft without `source.consumerGroup` keeps the previous draft's group (routes `DraftEditor.KeepConsumerGroup`):
   a changed group means a new group without committed offsets, which re-reads the topic from `startFrom`.
9. Chaos and outage tests use in-process fault injection on kfake (broker listener stop/start, per-worker dial
   cut) instead of Toxiproxy: the shared dev Kafka advertises a fixed listener, so a proxy cannot sit in between.

10. [P2] Branch labels: record-level series carry the primary (first) branch, `produced` and branch failures carry
    their own branch (see 2.2), so route-level sums used by alert rules count each record once.
11. [P2] Producer sharing is on by default; shared clients lift the franz-go buffered-record cap because every
    runner bounds its own in-flight records.
12. [P2] Runtime events for alerting stay on the 0008 `router_runtime` NOTIFY (`kind=runtime` on every
    `route_runtime` change incl. halt); alert rules are metric based (lag, error rate, throughput, match rate), so
    route-runtime keeps those inputs correct (lag probe, revoked-partition lag reset, one count per record).
13. [P2] An `output_invalid` caused by an unreachable dependency is `destination_transient` (codec errors signal it
    with `Temporary() bool`), so a schema-registry outage never dead-letters good records.
14. [P3] Exactly-once uses one `GroupTransactSession` per runner as both source consumer and producer (never pooled);
    one transaction per poll, the poll waits for every record of its batch (`BatchBarrier`); a failed transaction is a
    recoverable, runner-local halt (`HaltReason.Recoverable()`): the worker restarts the runner, the route is not paused.
15. [P3] Replay suspends the route through the scheduler (`RouteDemand.Replaying` makes it unschedulable) instead of
    pausing it, so `desired_state` and the halt bookkeeping are untouched; the reset uses connections' `pkg/kafka`
    admin (`DescribeGroup`, `OffsetsForTimes`, `CommitOffsets`, CN-205) with specs resolved in the API.
16. [P3] `ReplayService` exposes `Reset(ctx, ReplayRequest{RouteID, From, RequestedBy})` and
    `Status(ctx, routeID, replayID)`; cmd/api hands it to routes with `apiService.runtimeReplays()`, where routes adapts
    it to its `OffsetResetter` port.
17. [P3] The live-tail port is `runner.TailTap{Active, Offer}`; `Active` must be a cached flag check so an untailed
    route pays nothing; monitoring's `Tap` plugs in through `cmd/worker/tail.go`.

## 11. Open questions (remaining)

1. Control-plane-down + worker restart: secrets are memory-only, so a restarted worker cannot start runners until
   the API returns. Accept, or allow an encrypted on-disk secret cache?
