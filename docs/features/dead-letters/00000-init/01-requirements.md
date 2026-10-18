# Dead letters — Requirements (00000-init)

Sources: UI spec §10.3 (Dead letters), §11.2 (roles/masking), §13 (phases); SysDesign §4 (DLQ not in Postgres),
§6 (dead-letter endpoints), §8.3 (error types), §8.4 (DLQ topic + resolved state), §11 (audit), §13 (testing), §14 (milestones).

## 1. Business intent
Failed events are never silently dropped: they land in a per-route Kafka DLQ topic with the reason.
This feature is the **read/act side** of that queue: users see what failed and why, fix the rule against the
exact failing event, then retry or discard — without Kafka tooling and without touching other consumers.

## 2. Functional requirements
Phase tags: **MVP** (milestone 2), **P2** (milestone 4), **P3** (milestone 5).

### Browse
- FR-1 [MVP] List a route's unresolved dead letters, newest first: time, error type, short message, step, source partition@offset.
- FR-2 [MVP] Global "Dead letters" screen lists dead letters across every route the user may read (read is open across teams), with route name.
- FR-3 [MVP] Cursor pagination (page ≤ 200); server scan bounded by time/bytes and reports `truncated`.
- FR-4 [MVP] Filter by error type and time range; [P2] by step, version and free text in the message.
- FR-5 [MVP] Summary counts (unresolved total, per error type) for the nav badge, route tab label and dashboard card.
- FR-6 [P2] Toggle "show resolved" to see retried/discarded events with who/when/outcome.
- FR-7 [MVP] DLQ topics of **all versions** of a route are browsed (errorPolicy.dlqTopic may change per version).

### Detail
- FR-8 [MVP] Detail shows original key, value, headers, failing step, error type, error message, JS stack trace,
  route version, source topic/partition/offset, failed-at time.
- FR-9 [MVP] JSON values shown decoded; non-decodable bytes shown as base64 with a "raw bytes" marker.
- FR-10 [P2] Avro/Protobuf values decoded through the source connection's schema registry.
- FR-11 [P2] Viewers always see masked values; other roles see masked values on connections marked `sensitive`; masks keep types (access-control `SampleVisibility`).

### Act
- FR-12 [MVP] Retry single or selected events: re-run through the route's **current active version** and produce to
  the destination; outcome per event = sent / filtered / failed (with new error). Sent + filtered → resolved.
- FR-13 [MVP] "Retry all" (selection = current filter) synchronously up to 500 events per call; response says how many remain.
- FR-14 [P2] Bulk retry/discard of any size as a background job with progress, cancel, and a bell notification on finish.
- FR-15 [MVP] Discard single/selected/all-matching events → resolved (state = discarded), hidden from the default list.
- FR-16 [P2] Restore (un-resolve) a discarded event within its retention window.
- FR-17 [MVP] Export selected or filtered events as a JSON file (max 10,000 per export); [P2] NDJSON format.
- FR-18 [MVP] "Open in Transform Studio" loads the failing event as a sample in the route's builder (transform step).
- FR-19 [P2] Branch-aware retry: only the branch that failed is re-emitted (no duplicates on healthy branches).
- FR-20 [P3] Demo workspace ships seeded dead letters on the fake `order-events` route.
- FR-21 [P3] Routes in `exactly_once` mode show that retries are delivered at-least-once (outside the consumer transaction).

### Governance
- FR-22 [MVP] Retry, discard and export require Editor (or Admin) on the route's team; browsing requires Viewer.
- FR-23 [MVP] Retry, discard, export written to `audit_log` with ids/counts and selection; [P2] raw-view also audited.

## 3. Non-functional requirements / constraints
- NFR-1 Dead letters are **not** copied into Postgres; the API reads DLQ topics on demand (SysDesign §4).
- NFR-2 Reads use manual partition assignment and **no consumer group** — browsing never commits offsets.
- NFR-3 Resolved state lives in the compacted topic `router.dlq.state` on the DLQ connection (SysDesign §8.4).
- NFR-4 Retry never writes to the source topic; it produces only to the route destination (idempotent, acks=all).
- NFR-5 Retry is at-least-once: a crash between produce-ack and state write can re-send on the next retry.
- NFR-6 One event cannot be retried concurrently twice (distributed lock per event, TTL bound).
- NFR-7 Per request limits: scan ≤ 10 s and ≤ 10 MB; list p95 < 1.5 s for a 1M-record DLQ topic with time filter.
- NFR-8 Payloads are never logged; secrets come only from `internal/secrets` Manager.
- NFR-9 Coding rules (.devlens/rules.md): base interfaces + Manager per extensible domain, config-selected
  implementations, enum/domain constants, class-based HTTP handlers, functions ≤ 50 lines.
- NFR-10 FE uses the service registry contract (`DeadLettersService`); mock and HTTP implementations are swappable.

## 4. Out of scope
- Writing DLQ records, DLQ topic auto-creation, `onError` policy behaviour → route-runtime.
- DLQ metrics/charts and "errors per minute" → monitoring. Replay from a timestamp → routes.
- Editing a payload before retry; retrying into a different destination; retry through a draft version.

## 5. Dependencies
- route-runtime: DLQ writer uses this feature's record contract (headers incl. `router.branch`, `router.stack`).
- platform-core: API conventions (cursor, RFC 7807 + path), auth middleware, audit middleware, store, Redis, HTTP client for web.
- pipeline-engine: `Compiler`/`Pipeline`, codecs (JSON MVP, Avro/Protobuf P2).
- connections: `pkg/kafka` client factory, admin (ListOffsets, watermarks), producer factory.
- live-preview: create a sample session from inline events (Open in Studio).
- transform-studio / routes: builder deep link with a preselected sample.
- access-control: RBAC permission checks, masking policy for sensitive connections, raw-data permission.
- monitoring: `WS /events` bus for job-finished notifications, dashboard dead-letter card.
