# Route Runtime — Requirements (00003-autotune)

Incremental version on top of `00000-init` and `00001-stress-test` (and `00002-memory-budget` when present, built
in parallel). **Unchanged:** FR-01..FR-36, NFRs, and the stress-test findings. This version adds only adaptive
in-flight sizing (fix #2 of `00001-stress-test/02-design.md` §7, bottleneck B2). The JS pool part of the same
feature (fix #5, B3) is documented in `js-sandbox/00001-pool-autoscale`.

## Business intent
The default `limits.maxInFlight` 1000 capped one 12-partition route at ~65k ev/s while CPU was idle (5000 → 80k).
Operators should not have to guess a number: the runtime sizes the in-flight window per route from what it
observes, and a configured number becomes a safety cap instead of a fixed size.

## Requirements
- AT-01 Each route runner has an in-flight limit that adapts between a floor and a cap from observed produce
  (ack) latency and queue wait (time a record waits for an in-flight slot).
- AT-02 `limits.inFlightMode`: `auto` (default) or `fixed`. In `auto`, `limits.maxInFlight` (when set) is the cap;
  unset → the auto cap default. In `fixed`, `limits.maxInFlight` (or the old default 1000) is the size — the
  pre-autotune behaviour.
- AT-03 `limits.minInFlight`: floor of the auto limit (default 64); must be ≤ the cap (validation error otherwise).
- AT-04 The auto limit starts at the previous default (1000, clamped into [floor, cap]).
- AT-05 The limit grows only when it binds (records wait for slots) and latency is within tolerance of the
  route's long-term latency; it shrinks when latency rises beyond tolerance.
- AT-06 Worker memory pressure (the worker byte budget of `00002-memory-budget`) always wins: while it signals
  pressure the limit only shrinks, never grows.
- AT-07 Per-partition ordering and at-least-once are unchanged: shrinking never drops or reorders records; slots
  already taken above a lowered limit drain naturally.
- AT-08 Metric `router_route_inflight_limit{route_id,version,worker}` exposes the current limit.
- AT-09 The route Settings tab shows the current (auto) values next to the limits being edited.
- AT-10 Hot reload (pipeline swap) applies changed limit bounds without restarting the runner.

## Out of scope
- Cross-route / worker-wide in-flight budgets (the byte budget covers memory).
- Changing lane queue marks (`LaneHighWater`, bytes marks) — owned by the memory budget work.

## Pass criteria
- Unit tests with a simulated clock and a latency model: converges to ≥ 90 % of the modelled capacity, never
  leaves [floor, cap], shrinks under latency rise and under pressure, holds when not binding.
- Ordering and no-loss test with the limit changing under load.
- Integration step (orchestrator): re-run S2 p12 with default limits → expect ≥ 80k ev/s (was 65k at mif 1000).
