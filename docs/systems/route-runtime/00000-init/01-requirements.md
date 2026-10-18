# Route Runtime — Requirements (00000-init)

Sources: SysDesign §8 (worker runtime), §9 (HA and lifecycle), §1 guarantees, §12 (metrics emitted), §13 (tests), §14 milestones.
Code: `cmd/worker`, `internal/runtime`, `internal/scheduler` (scheduler runs inside `cmd/api`, leader-only).

## Business intent
Routes the user deploys must actually run against any registered Kafka cluster: consume, filter/transform via the
shared engine, produce, never lose data, survive worker crashes and control-plane outages, and pick up new versions
without restarts. Reliability is the product's promise; this system is where that promise is kept or broken.

## Phase tags
MVP = SysDesign milestones 2 (single worker) + 3 (HA); P2 = UI spec Phase 2; P3 = UI spec Phase 3.

## Functional requirements
### Processing loop (§8.1)
- FR-01 [MVP] One route runner per assigned route: one consumer on the source connection (group = `source.consumerGroup`), one compiled `engine.Pipeline`, producers per destination connection.
- FR-02 [MVP] Per-partition in-order processing; partitions run in parallel, total in-flight bounded by `limits.maxInFlight`.
- FR-03 [MVP] Per record: decode → filter → transform → encode → async produce (idempotent, `acks=all`).
- FR-04 [MVP] Per partition track the highest offset whose record and all earlier records are done (acked, filtered, dropped, DLQ'd, skipped).
- FR-05 [MVP] Commit done offsets every 1 s (domain default) and on revoke, rebalance, pipeline swap and shutdown.
- FR-06 [MVP] Honour `source.startFrom` (latest / earliest / timestamp) only when the group has no committed offset; support `topicPattern`.
- FR-07 [P2] Multi-branch routes: each `Out.Branch` goes to its branch destination; a record is done only when every output of every branch is acked.
- FR-08 [P2] Producer clients shared between routes on the same destination connection when credentials match.
### Exactly-once (§8.2)
- FR-09 [P3] `delivery.mode=exactly_once` (same connection only): transactional producer, outputs + DLQ writes + offsets in one transaction per batch; abort rewinds to last committed offsets.
### Error handling and DLQ write (§8.3, §8.4)
- FR-10 [MVP] Classify every failure into a domain error category: decode, step, step_timeout, output_invalid, destination_transient, destination_permanent, source_fatal, source_transient.
- FR-11 [MVP] Per-event categories follow `errorPolicy.onError`: `dlq` (default) / `skip` (drop + count) / `pause` (halt route).
- FR-12 [MVP] Step timeouts retried in memory up to `errorPolicy.maxRetries` before the policy applies; other step errors are not retried.
- FR-13 [MVP] Destination transient: exponential backoff 100 ms → 30 s, pause fetch on affected partitions, no commit past the failed record, no loss.
- FR-14 [MVP] Destination permanent / source fatal: stop runner, report observed `error` with message, halt the route (no restart loop), raise runtime event for alerts.
- FR-15 [MVP] DLQ record = original key, value bytes, headers + `router.route_id`, `router.version`, `router.step`, `router.error_type`, `router.error_message`, `router.source_topic`, `router.source_partition`, `router.source_offset`, `router.failed_at`.
- FR-16 [MVP] DLQ topic (`errorPolicy.dlqTopic`, default `router.dlq.<route>`) auto-created (retention 7 days) on `errorPolicy.dlqConnectionId`, default the source connection; DLQ produce failure is treated as destination transient.
- FR-17 [P2] Output schema validation against destination Avro/Protobuf schema → `output_invalid` → DLQ.
### Backpressure and limits (§8.5)
- FR-18 [MVP] Bounded in-flight queue per runner; when full or producer slow, pause fetching on affected partitions instead of buffering.
- FR-19 [MVP] Report per-route CPU time and memory estimate; worker refuses assignments above its capacity score.
### Scheduler and leases (§9.1)
- FR-20 [MVP] Scheduler runs in the API; exactly one active instance via Postgres advisory lock; others standby.
- FR-21 [MVP] Every route with `desired_state=running`, not halted, with an active version gets `replicas` workers (default 2, capped at source partition count).
- FR-22 [MVP] Placement picks lowest load score, spreads replicas across hosts and zones; strategy selected by config.
- FR-23 [MVP] Assignments are leases in `route_assignments` (TTL 30 s), renewed by worker heartbeats every 10 s; expired leases are reassigned.
- FR-24 [MVP] Paused / archived / halted routes lose their assignments; observed state becomes `paused` once all runners confirm stop.
### Worker reconciliation (§9.2)
- FR-25 [MVP] Each worker reconciles desired (leases + target version) vs actual runners: start missing, stop unleased, upgrade changed.
- FR-26 [MVP] Change notifications via Postgres `LISTEN/NOTIFY` relayed by the API; full resync every 15 s as safety net.
### Hot reload and rollout (§9.3, §9.4 runtime side)
- FR-27 [MVP] New version compiled in background; compile failure keeps old version running and marks deployment `failed`.
- FR-28 [MVP] Filter/transform/limits-only change: stop fetch, drain in-flight, commit, swap pipeline, resume — without leaving the consumer group.
- FR-29 [MVP] Source (connection, topics, pattern, group) / destination connection / delivery mode change: full runner restart.
- FR-30 [MVP] Replicas upgrade one at a time; deployment `active` when all replicas report the new version; `superseded` for the previous one.
- FR-31 [MVP] New version erroring within 60 s of activation → deployment `failed`, automatic rollback that creates a new version copying the previous one (routes `DeploymentLifecycle.MarkFailed`, origin `auto_rollback`).
### Control plane down (§9.5)
- FR-32 [MVP] Worker caches assignments + version configs locally; if the control plane is unreachable it keeps running current routes past lease expiry.
- FR-33 [MVP] A worker stops a route only on an explicit control-plane answer (assignment absent), never on silence.
### Replay and live tail hook
- FR-34 [P3] Replay mechanism (`OffsetResetter` for routes' `POST /routes/{id}/replay`): stop runners, wait for empty group, reset group offsets to the timestamp, resume; audited. API owned by routes, confirm dialog by monitoring.
- FR-35 [P3] Runner tap: when the live-tail flag is set, copy ≤ 20 events/s (input + output) to the tail sink; zero cost otherwise.
### Observability emitted
- FR-36 [MVP] Emit §12 worker metrics labelled `route_id, branch, version, worker`; lag = high watermark − committed offset.
- FR-37 [MVP] Report per-replica status (state, version, error, lag) to the control plane; API aggregates into `route_runtime`.

## Non-functional / constraints
- At-least-once: an offset is never committed before its outputs are durable; duplicates only from the uncommitted window.
- Ordering preserved per source partition; per-key order with `by_key` partitioning.
- Isolation: one route's failure, slowness or JS limits never stall other runners on the same worker.
- Worker is stateless except an on-disk config cache; secrets live only in memory, never logged; payloads never logged.
- Recovery from worker crash within Kafka rebalance time (seconds); hot reload causes no rebalance for transform-only changes.
- Coding rules (`.devlens/rules.md`): base interface + Manager per extensible domain, config-selected implementations, enums/domain constants.

## Out of scope
Stateful processing (joins/windows), non-Kafka sinks, cross-cluster exactly-once, DLQ browse/retry/discard
(features/dead-letters), metric storage/alert evaluation/tail relay (features/monitoring), deploy API (features/routes).

## Dependencies
- systems/pipeline-engine: `engine.Compiler`, `Pipeline`, `CodecManager`, `config` types and change classification inputs.
- systems/js-sandbox: runtime pools per route, limit errors typed as `step_timeout` / step errors.
- systems/platform-core: migrations, internal workload-identity auth, internal secrets endpoint, NOTIFY plumbing, audit middleware.
- features/connections: `pkg/kafka` client factory per security mode, admin ops (create topic, list offsets, group ops).
- features/routes: deploy transaction (version + `active_version_id` + deployment row), pause/resume/rollback endpoints.
- features/monitoring: metric catalogue names, live-tail sink + Redis flag, runtime events on `WS /events`, alerts.
