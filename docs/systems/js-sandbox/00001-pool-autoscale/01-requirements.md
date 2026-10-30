# JS Sandbox — Requirements (00001-pool-autoscale)

Incremental version on top of `00000-init`. **Unchanged:** every requirement of `00000-init/01-requirements.md`
(isolation, limits, determinism, host functions, APIs). This version adds only pool autoscaling, the fix #5 /
bottleneck B3 of `route-runtime/00001-stress-test/02-design.md`. The in-flight part of the same feature lives in
`route-runtime/00003-autotune`.

## Business intent
Every JS step had a fixed pool of 4 instances per route version (`DefaultSandboxParallelism`), so a JS route
peaked at ~5k ev/s with 1.5–1.8 of 6 cores idle. The pool should grow while events queue for an instance and the
worker has CPU and JS memory left, and give instances back when the route is idle.

## Requirements
- PA-01 Each worker JS pool has a current size between `limits.jsMinInstances` and `limits.jsMaxInstances`.
- PA-02 Defaults: min 1; start 4; max `auto` = worker cores (GOMAXPROCS, at least 4), bounded by the operator maximum
  `ROUTER_SANDBOX_POOL_MAX_INSTANCES` (default 32). A route value above the operator maximum is clamped.
- PA-03 Grow (+1 or +25 %, whichever larger) when calls wait for an instance (queue wait above the threshold),
  worker CPU is below the CPU ceiling, and the worker JS memory ceiling has room for one more instance.
- PA-04 Shrink by one instance after a run of idle windows (peak in-use below the size); never below min.
- PA-05 Memory: each instance reserves its per-instance cap (`limits.jsMemoryMb`) from a worker-wide JS memory
  ceiling `ROUTER_SANDBOX_MEMORY_CEILING_MB` (default 4096); growth that does not fit is refused. Warm instances
  of fixed pools reserve too, but are never refused (they existed before autoscaling).
- PA-06 Preview, test and DLQ-retry scopes keep their fixed single instance (min = max = 1).
- PA-07 Metrics: `router_js_pool_instances{route_id,version}` (summed over the version's pools),
  `router_js_queue_wait_seconds{route_id,version}` histogram. Every `router_js_*` series labels `version` with the
  route version number (as the route series do), not the version UUID.
- PA-08 Validation: `jsMinInstances` ≤ `jsMaxInstances`; both 1..256 in the schema.
- PA-09 Settings tab shows the current instance count next to the JS pool limits.

## Pass criteria
- Unit tests with a simulated clock: grows under queue wait, holds at CPU / memory ceilings, shrinks when idle,
  never leaves [min, max], no call lost during shrink.
- Benchmark: a JS step under parallel load scales throughput with the instance count.
- Integration step (orchestrator): re-run S5 JS-only → expect > 5k ev/s, scaling toward ~1.25k × cores.
