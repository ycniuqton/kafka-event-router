# Route Runtime — Design (00003-autotune)

Incremental version. **Unchanged:** everything in `00000-init/02-design.md` and `00001-stress-test/02-design.md`
(packages, lanes, offset tracking, commits, error policies, swap, lane backpressure marks, heartbeat) except the
in-flight limiter of flow 5.1, which becomes adaptive. The worker byte budget of `00002-memory-budget` (built in
parallel) plugs in through the `MemoryPressure` port below. JS pool autoscaling: `js-sandbox/00001-pool-autoscale`.

## 1. What changed

| Piece | File (internal/runtime/runner unless noted) | Change |
|---|---|---|
| Limits | `pkg/config/schema/route.v1.schema.json`, `pkg/config/autotune.go` | `limits.inFlightMode` auto/fixed, `limits.minInFlight`; `Limits.InFlight()` → `InFlightBounds{Mode, Min, Max, Start}` |
| Validation | `pkg/config/validators/limits.go` | new `limits` validator (draft/deploy/offline): floor > cap → error, floor in fixed mode → warning |
| Limiter | `inflight_limiter.go` | channel semaphore → resizable FIFO semaphore (`SetLimit`), window stats (queue wait, ack latency, peak) |
| Controllers | `inflight_controller.go`, `inflight_gradient.go` | `InFlightController` base, `FixedInFlight`, `GradientInFlight`, `InFlightControllers` registry by mode |
| Loop | `inflight_tuner.go` | `InFlightTuner` per runner (1 s window), `MemoryPressure` port, `InFlightGauge` |
| Wiring | `runner.go`, `runner_lifecycle.go`, `record_flow.go` | tuner built in `Start`, run with the poll loop, `Rebind` on `Swap`; ack latency observed on the last ack |
| Metric | `metrics.go` | `router_route_inflight_limit{route_id,version,worker}` (deleted with the runner's series on stop) |
| Overview | `internal/metrics/overview`, `query/catalog.go` | `GET /routes/{id}/metrics` → `tuning {inFlightLimit, jsInstances, jsQueueWaitP95}` |
| UI | `web/.../settings/LimitsSection.tsx` | Auto-tune / Fixed switch, floor + JS pool fields, "Now: …" line from `tuning` |

## 2. Bounds

| `inFlightMode` | `maxInFlight` | `minInFlight` | Floor | Cap | Start |
|---|---|---|---|---|---|
| absent / `auto` | absent | absent | 64 | 10 000 | 1000 |
| `auto` | N | M | min(M, N) | N | clamp(1000, floor, N) |
| `fixed` | absent / N | ignored (warning) | 1000 / N | 1000 / N | 1000 / N |

Routes that set `maxInFlight` without a mode (e.g. the stress routes at 5000) become auto with that cap.

## 3. Controller (`GradientInFlight`, constants `DefaultGradientTuning`)

Per window: `short` = mean ack latency (dispatch → last ack), `long` = EMA of `short` (α 0.05 rising, 0.25
falling), gradient `g = clamp(2.0 × long / short, 0.5, 1)`, binding = ≥ 1 % of admitted records waited for a slot.

```
tuner tick (every 1 s, per runner)                                          [cron]
└─ limiter.TakeWindow → acquired, waited, wait time, acks, ack latency, peak
└─ MemoryPressure.UnderPressure?
   └─ yes → limit × 0.5 (floor), hold 10 windows        → byte budget always wins
   └─ no
      └─ acks < 32                                     → hold (idle / too few samples)
      └─ g < 1 (latency > 2 × long-term)
         └─ peak in use ≥ 50 % of the limit, limit > start → max(limit × g, start), hold 10 windows
         └─ otherwise (limit mostly idle)               → hold, cancel probe (spike is not this route's)
      └─ previous window grew the limit (probe open)
         └─ not binding any more                        → keep (enough slots)
         └─ throughput +≥ 3 %                           → keep, grow again × 1.25
         └─ otherwise                                   → revert to the pre-growth limit, hold 10 windows
      └─ holding                                        → hold
      └─ binding                                        → limit × 1.25 (≥ +1), open probe
      └─ otherwise                                      → hold
   └─ clamp [floor, cap] → limiter.SetLimit → router_route_inflight_limit
---
lane handles record                                                         [event]
└─ limiter.Acquire (FIFO; waits while inUse ≥ limit; wait time → window)
└─ process → deliver → produce [async]
   └─ last ack → limiter.ObserveAck(since dispatch) → finish → limiter.Release → wake oldest waiter
---
pipeline swap (hot reload)                                                  [event]
└─ tuner.Rebind(new bounds) → new controller continues from the current limit, clamped into the new bounds
```

Why the usage check (wave-2 integration fix): without it a lightly used limit shrank on every broker latency
spike but only grew when binding, so it ratcheted to the floor (64) on a quiet route; S2 p1 then capped at ~26k
(64 ÷ ~2.5 ms) with the worker 0.5 cores busy. Shrinking an idle limit cannot lower latency, so it now holds.

Why the start floor (second wave-2 fix): even with the usage check, broker latency blips while the limit binds
shrank it 1000 → 64; throughput then fell below the offered load, lag grew and the +3 % probes never paid off (a
death spiral: S2 confirm p12 10k, p1 20k). Latency back-off now stops at the start limit (1000 by default = the
old fixed size), so auto never runs below the fixed default; only memory pressure takes the limit to the floor.

Why the probe: with latency that rises with load (`lat = base + inflight / capacity`), the latency gradient alone
lets the long-term EMA creep upward; the throughput check stops growth once 25 % more slots buy < 3 % more
throughput. With the broker as bottleneck (latency ∝ in-flight) every probe is reverted.

Ordering / no loss: a lane handles its records one after another and blocks in `Acquire`, so a resize never
reorders a partition. Lowering the limit revokes nothing — slots above it drain on `Release`; waiters are served
in arrival order; a cancelled waiter leaves the queue or hands a late grant back.

## 4. Integration with the memory budget (F1, `00002-memory-budget`)
Hook point: `runner.Deps.InFlight.Pressure` (`MemoryPressure` interface, default `NoMemoryPressure`). The worker
assembly passes the budget there; when it reports pressure the limit halves per window down to the floor and
does not grow (§3). The budget's own fetch pausing is unchanged and independent.
Wired at integration: `membudget.Budget.UnderPressure()` = worker usage ≥ high watermark **or** any route still
paused by the budget (so pressure clears with the same low-watermark hysteresis as the pauses);
`cmd/worker` `Assembly.inFlightTuning(budget)` sets `Deps.InFlight.Pressure`. `InFlightTuning.Controllers`
and `.Interval` are the other knobs (tests use 5 ms windows).

## 5. Numbers (unit model, `inflight_controller_test.go`, deterministic, ±1 % jitter)

| Model | Start | Result |
|---|---|---|
| CPU 80k ev/s, ack 15 ms, backlog (S2 p12 shape) | 1000 → 66k | 1250 after 1 window → 80.2k (CPU-bound, stops: no queueing) |
| latency = 5 ms + inflight / 85k | 1000 → 59k | 2938 → 74k after 5 s; 5739 → 79k after 300 s |
| broker 60k, ack 15 ms | 1000 → 60k | probes reverted, stays ≤ 1600 over 300 s |
| demand 10k (not binding) | 1000 | holds 1000 |
| pressure from window 4 | — | halves to the floor, never grows |

Limiter cost: 292 ns per acquire + ack + release with 72 contending goroutines (`BenchmarkInFlightLimiter`).

## 6. Decisions
- D-AT-1 Auto is the default (requirement); fixed mode keeps the exact pre-autotune behaviour.
- D-AT-2 Auto cap default 10 000 (2× the best stress value 5000); memory is bounded by the byte budget, not this cap.
- D-AT-3 The tuner runs per runner (1 s); no cross-route coordination — the byte budget is the shared resource.
- D-AT-4 `tuning` values are sums over replicas (per-runner limits add up to the route's window).

## 7. Integration checks (orchestrator, not run here)
- S2 p12 1 KB with default limits: expect ≥ 80k ev/s (was 65k at mif 1000); `router_route_inflight_limit` above 1000.
- S2 p12 with `maxInFlight` 1000 (now an auto cap): ≈ the old 65k; with 5000: ≈ 80k.
- S2 mif 100: expect ≈ 8k (cap honoured).
- S4 fan-out / S7 failure cases with the memory budget merged: limit shrinks under pressure, 0 lost.
