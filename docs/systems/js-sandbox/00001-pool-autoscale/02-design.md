# JS Sandbox — Design (00001-pool-autoscale)

Incremental version. **Unchanged:** architecture, isolation, host functions, determinism, error cases and APIs
of `00000-init/02-design.md`. Changed: pool sizing (§2.1 `Pool` / `BoundedPool`, flow "load" step
`BoundedPool per Program (max = parallelism)`), the `Observer` port, `RouteScope`, service configuration (§4),
and the `router_js_*` label values.

## 1. What changed

| Piece | File (pkg/sandbox unless noted) | Change |
|---|---|---|
| Scope | `module.go` | `RouteScope.VersionNo`, `RouteScope.Pool *PoolRequest{Min, Max}` (nil = fixed at `Parallelism`); `VersionLabel()` |
| Sizing | `pool_sizing.go` | `PoolSizing.For(scope)` → start + `PoolBounds`; Max 0 = max(GOMAXPROCS, 4); operator cap; only worker scopes autoscale |
| Pool | `pool.go`, `pool_scale.go` | channel slots → resizable FIFO slots; `ScalablePool{Bounds, Resize, TakeWindow}`; idle cap = capacity when autoscaled |
| Memory | `ceiling.go` | `MemoryCeiling`: every slot reserves the per-instance cap; starting slots forced, growth refused when full |
| Scalers | `pool_scaler.go` | `PoolScaler` base, `FixedPoolScaler`, `QueueWaitScaler`, `PoolScalers` registry by `PoolBounds.Mode()` |
| CPU | `cpu.go`, `cpu_unix.go`, `cpu_other.go` | `CPUGauge` port, `ProcessCPU` (getrusage ÷ wall × GOMAXPROCS), unknown elsewhere |
| Loop | `autoscale.go`, `manager_scale.go`, `manager.go` | `PoolAutoscaler` started by `NewManager`, stopped by `Close`; Manager implements `ScalablePools` |
| Metrics | `observe.go`, `route_sandbox.go` | `router_js_queue_wait_seconds`; pool gauges summed per route version; `version` = version number |
| Worker | `internal/runtime/reconcile/compiler.go` | `WorkerScope.Of` (version no + `limits.jsMinInstances/jsMaxInstances`, start 4) |
| Settings | `internal/sandboxinfo/settings.go` | `ROUTER_SANDBOX_POOL_MAX_INSTANCES` (32), `ROUTER_SANDBOX_MEMORY_CEILING_MB` (4096) |
| Schema | `pkg/config` | `limits.jsMinInstances`, `limits.jsMaxInstances` (1..256), validator min ≤ max |

## 2. Sizing

| Scope | Start | Bounds |
|---|---|---|
| worker, no JS limits | 4 | [1, min(max(cores, 4), 32)] |
| worker, `jsMinInstances` m, `jsMaxInstances` M | clamp(4, m, M′) | [m, M′], M′ = min(M, 32) |
| worker, m = M | m | fixed |
| preview / test / DLQ retry (Parallelism 1, no request) | 1 | fixed |

## 3. Flows

```
autoscaler tick (every 1 s, one loop per worker sandbox Manager)                 [cron]
└─ CPUGauge.Busy → process CPU share of GOMAXPROCS since the last tick
└─ for each pool of each open route sandbox with autoscaled bounds
   └─ pool.TakeWindow → calls, waited, wait time, peak in use, size
   └─ QueueWaitScaler.Next
      └─ ≥ 5 % of calls waited for an instance
         └─ CPU < 0.85 or unknown          → size + max(1, 25 %)
         └─ CPU ≥ 0.85                     → hold
      └─ peak in use = size                → hold, reset idle streak
      └─ peak < size for 30 windows        → size − 1
   └─ pool.Resize(target)
      └─ clamp [min, max]
      └─ grow   → reserve one per-instance cap per slot from MemoryCeiling; stop at the first refusal → wake waiters
      └─ shrink → free reservations, close idle surplus; busy surplus closes on Release
└─ PublishUsage → router_js_pool_instances / memory summed per route version
---
JS call (lane → step → Module.Call)                                              [event]
└─ pool.Acquire: free slot → take; else queue FIFO (wait counted in the window) until slot or ctx deadline
└─ observer.QueueWaitObserved(acquire time) → router_js_queue_wait_seconds
└─ invoke → Release (keep idle while size ≤ capacity and idle < cap) / Discard
---
route version closes (swap / stop)                                               [event]
└─ each step sandbox closes → pool.Close frees every reservation
└─ Manager.forget → other sandboxes of the version open → republish sums; last one → ScopeClosed (gauges 0)
```

Shrinking never cancels a call: capacity only limits new grants; in-use instances finish and close on release.

## 4. Metrics and labels
All `router_js_*` series: `route_id` = route id, `version` = route version number (`RouteScope.VersionNo`; falls
back to the version id for unnumbered scopes). Before: `version` carried the version UUID and every JS step's
sandbox overwrote `router_js_pool_instances` (last writer won); now the Manager sums every step of the version.
New: `router_js_queue_wait_seconds{route_id,version}` histogram (10 µs … 0.5 s).

## 5. Numbers
`BenchmarkJSPoolInstances` (typical 20-line transform, 1 KB-ish event, `-cpu 6`, 12+ concurrent callers, shared host):

| Instances | 1 | 2 | 4 | 6 | 8 | 12 |
|---|---|---|---|---|---|---|
| calls/s | 2.6k | 5.2k | 7.9k | 13.7k | 12.5k | 12.3k |

Throughput scales with instances up to the cores, then flattens: the old fixed 4 left ~40 % on the table on 6
cores. Auto max = cores (at least the old 4) is the right default; the CPU ceiling (0.85) keeps room for the
route's codec/GC.

## 6. Decisions
- D-PA-1 Memory ceiling reserves the per-instance cap, not the measured size: deterministic, no OOM risk.
  Default 4 GiB = 64 instances at the 64 MB default cap.
- D-PA-2 Starting slots are forced past the ceiling so existing routes keep working; only growth is refused.
- D-PA-3 One autoscaler loop per Manager (worker-wide), not per pool: one CPU sample, one gauge publish per tick.
- D-PA-4 Shrink is slow (−1 per 30 idle windows) and growth fast (+25 %): idle instances cost memory only.

## 7. Integration checks (orchestrator, not run here)
- S5 JS-only (12 p, 1 KB): expect > 5k ev/s (was 5k at 4 instances), `router_js_pool_instances` at ~cores per JS step.
- S5 filter 10 + steps 5 + JS: expect > 4k.
- Staging (1 CPU): auto max = 4 (never below the old fixed size), no regression.
- Grafana / alert queries on `router_js_*{version=…}` must use version numbers now.
