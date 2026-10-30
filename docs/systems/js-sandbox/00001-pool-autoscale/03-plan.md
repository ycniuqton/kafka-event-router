# JS Sandbox — Plan (00001-pool-autoscale)

Incremental version. **Unchanged:** every item of `00000-init/03-plan.md` keeps its state there. This plan adds only
the pool autoscaling items (`JS-PA-xx`). Mark the step you start `[~]`, finished `[x]`; never delete items.

## Config
- [x] JS-PA-01 Schema `limits.jsMinInstances`, `limits.jsMaxInstances`; regenerate Go + TS types
- [x] JS-PA-02 Validator: min ≤ max; `ROUTER_SANDBOX_POOL_MAX_INSTANCES`, `ROUTER_SANDBOX_MEMORY_CEILING_MB` settings
- [x] JS-PA-03 `RouteScope.Pool` (min/max) built by the worker compiler from route limits (auto max = cores)

## Pool
- [x] JS-PA-10 `BoundedPool` resizable capacity (`Resize`, idle cap follows size, queue-wait stats per window)
- [x] JS-PA-11 `MemoryCeiling` worker-wide reservation of per-instance caps
- [x] JS-PA-12 `PoolScaler` base + `FixedPoolScaler` / `QueueWaitScaler`; `CPUGauge` (process rusage) port
- [x] JS-PA-13 `PoolAutoscaler` loop in the Manager (one ticker for every open worker pool), stopped on Close

## Metrics
- [x] JS-PA-20 `router_js_queue_wait_seconds` histogram; `router_js_pool_instances` summed per route version
- [x] JS-PA-21 `version` label = version number (RouteScope.VersionNo) for every `router_js_*` series

## Tests / bench
- [x] JS-PA-30 Scaler tests on a simulated clock (grow, CPU ceiling, memory ceiling, shrink, bounds)
- [x] JS-PA-31 Pool resize tests: no call lost while shrinking; waiters woken on grow
- [x] JS-PA-32 Benchmark: JS step throughput vs instance count

## Docs / integration
- [x] JS-PA-40 `02-design.md` final
- [x] JS-PA-41 Integration (orchestrator — not run by F5: no integration/e2e allowed): re-run S5 JS-only and filter+steps+JS — done for JS-only (wave 2): 6.5k ev/s fast and tree (wave 1: 5k with 4 instances), worker CPU 5.9 of 6 cores (pool now CPU-bound, not instance-bound); filter+steps+JS not re-run
