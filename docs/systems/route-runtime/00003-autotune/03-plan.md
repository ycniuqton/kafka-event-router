# Route Runtime — Plan (00003-autotune)

Incremental version. **Unchanged:** every item of `00000-init/03-plan.md` and `00001-stress-test/03-plan.md` keeps
its state there. This plan adds only the adaptive in-flight items (`RR-AT-xx`). The JS pool items are in
`js-sandbox/00001-pool-autoscale/03-plan.md`. Mark the step you start `[~]`, finished `[x]`; never delete items.

## Config
- [x] RR-AT-01 Schema `limits.inFlightMode` (auto|fixed), `limits.minInFlight`; regenerate Go types (go-jsonschema) and TS (`cmd/gen-tsschema` + json-schema-to-typescript)
- [x] RR-AT-02 `Limits.Resolved()` → `InFlightBounds` (mode, floor, cap, start); defaults in `pkg/config/types.go`
- [x] RR-AT-03 Validator: `minInFlight` ≤ cap (error), fixed mode ignores floor (warning)

## Runtime
- [x] RR-AT-10 `InFlightLimiter` becomes resizable (FIFO waiters, `SetLimit`, queue-wait + ack-latency window stats)
- [x] RR-AT-11 `InFlightController` base + `FixedInFlight` / `GradientInFlight` (latency-gradient AIMD) + `InFlightControllers` registry by mode
- [x] RR-AT-12 `InFlightTuner` loop per runner (interval, injectable ticker) + `MemoryPressure` hook for the worker byte budget (F1)
- [x] RR-AT-13 Record flow feeds ack latency; runner launch / swap wire the tuner and its bounds
- [x] RR-AT-14 Metric `router_route_inflight_limit{route_id,version,worker}` (set per tick, cleared on stop)

## Monitoring / UI
- [x] RR-AT-20 `GET /routes/{id}/metrics` returns `tuning` (current in-flight limit, JS instances, JS queue wait p95)
- [x] RR-AT-21 Settings tab Limits section: in-flight mode + floor fields, current auto values shown

## Tests / bench
- [x] RR-AT-30 Controller tests on a simulated latency model (converge, cap, floor, shrink, pressure, hold)
- [x] RR-AT-31 Limiter tests: resize under load, FIFO, ctx cancel, ordering preserved, no loss
- [x] RR-AT-32 Config + validator tests; web tests for the new fields

## Docs / integration
- [x] RR-AT-40 `02-design.md` final (flows, numbers)
- [x] RR-AT-41 Integration (orchestrator — not run by F5: no integration/e2e allowed): re-run S2 p12 default limits and mif 1000 / 5000 caps — done (wave 2): p12 default auto 140k fast / 80k tree (wave 1 fixed 1000: 65k), cap 5000 140k / 80k; p1 default exposed the ratchet / death spiral fixed in RR-AT-43/44 (20k → 80k fast); results `test/stress/results/wave2-s2-*.json`
- [x] RR-AT-42 Integration: worker budget wired as the tuner's pressure (`Budget.UnderPressure`, `Assembly.inFlightTuning`) — tests `membudget/budget_test.go` (`TestBudgetUnderPressureFollowsHighWatermarkAndPauses`), `cmd/worker/assembly_test.go`
- [x] RR-AT-43 Integration fix: back off only when the route fills ≥ 50 % of its limit (`GradientTuning.UsageRatio`, `GradientInFlight.backOff`); a quiet route no longer ratchets to the floor on broker latency spikes (S2 p1 default 20k → see wave-2 results); test `TestGradientIgnoresLatencySpikesOfAnUnusedLimit`
- [x] RR-AT-44 Integration fix: latency back-off never goes below the start limit (only memory pressure reaches the floor) — `GradientInFlight.backOff`; tests `TestGradientLatencyNeverDropsBelowTheStart`, `TestGradientShrinksOnLatencyRise` (now from a grown limit)
