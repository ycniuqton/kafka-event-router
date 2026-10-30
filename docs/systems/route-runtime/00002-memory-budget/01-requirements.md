# Route Runtime — Requirements (00002-memory-budget)

Incremental version on top of `00001-stress-test`. **Unchanged:** FR-01..FR-36 and the NFRs of `00000-init`, and
the stress-test goals of `00001-stress-test`. This version implements follow-up RR-OPT-03 (bounded worker memory).

## Business intent
Lane caps (2048 records / 2 MiB per partition) bound one partition, not the worker: with hundreds of routes and
partitions, or a slow destination, buffered records grow until the container is OOM-killed (S3, overload). A
worker must degrade by slowing consumption (lag grows) instead of dying, and must never lose or reorder events.

## Functional requirements
- MB-01 One worker-wide **memory budget** in bytes bounds the records held by all runners: queued in lanes plus
  dispatched and not yet finished (acked, filtered, dead-lettered, abandoned).
- MB-02 Record cost = key + value + header bytes + a fixed per-record overhead.
- MB-03 Budget size: `ROUTER_WORKER_MEMORY_BUDGET_BYTES` if set; else `ROUTER_WORKER_MEMORY_BUDGET_PERCENT`
  (default 60) of the memory limit; the memory limit is `GOMEMLIMIT` if set, else the container cgroup limit
  (v2 `memory.max`, v1 `memory.limit_in_bytes`); without any limit a fixed fallback of 1 GiB is used.
- MB-04 When `GOMEMLIMIT` is unset and a cgroup limit exists, the worker sets the Go soft memory limit to
  `ROUTER_WORKER_GOMEMLIMIT_PERCENT` (default 90) of the cgroup limit at start-up and logs it.
- MB-05 **Fair share**: while total usage is at or above the high watermark (100 % of the budget), a route whose
  usage is at or above its fair share (budget / active routes) has fetching paused on its source consumer
  (franz-go `PauseFetchTopics`); routes below their share keep fetching. A single route never exceeds the budget.
- MB-06 A paused route resumes when total usage drops to the low watermark (80 % of the budget) or its own usage
  drops below half of its fair share; resume is re-evaluated whenever usage drops or a route joins/leaves.
- MB-07 Pausing only stops fetching: records already handed to lanes are processed; records stripped from the
  client buffer are re-fetched from the same offset, so no event is dropped or reordered and commits are unchanged.
- MB-08 The budget pause is independent of lane pauses (`queue_full`) and producer backoff pauses; each releases
  only its own hold.
- MB-09 A stopped / revoked runner releases all its usage at once (queued records dropped, in-flight abandoned).
- MB-10 Metrics: `router_worker_memory_budget_bytes`, `router_worker_memory_used_bytes`,
  `router_worker_memory_paused_routes` (gauges), `router_worker_memory_pauses_total` (counter); label `worker`.

## Non-functional
- Accounting is lock-free on the record path (atomics); pause/resume transitions take a short lock.
- Unit tests: fair share decisions, hysteresis, join/leave re-evaluation, cgroup parsing, settings; a runner test
  with kfake proving fetching pauses over budget, resumes under the low watermark and every record arrives once
  in order.

## Out of scope
- Accounting decoded/transformed record memory (JS heaps, encoded outputs) — covered by the % margin.
- Per-route configurable budget weights.
