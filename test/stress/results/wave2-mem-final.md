# wave2-mem (local)

Wave-2 memory overload: worker started with GOMEMLIMIT=512MiB and ROUTER_WORKER_MEMORY_BUDGET_BYTES=268435456 (256 MiB). 25 routes fan out one 12-partition input at 5k ev/s (output 125k ev/s, more than one worker can produce: the destinations are the slow side; in wave 1 this case grew RSS to 4.8 GB). Pass = no crash/OOM, RSS under 512 MiB, lost = 0, router_worker_memory_paused_routes > 0. Delivery tracked per output topic.


Run `stress-wave2-101816290551` on vps-c0215fee (14 CPUs), 2026-10-18T14:29:05-02:00 → 2026-10-18T14:35:54-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| fanout25 | 25 | 5000 | 0 |  | 299.0 | 0 | 0 | 5099 | 4.69 |  |
| fanout50 | 0 | 0 | 0 |  | 0.0 | 0 | 0 | 0 | 0.00 | 6/50 routes failed: route d2e3a749-094f-483a-80dc-54a92245b972 not running after 3m0s; route ef1f2308-4209-4413-a7b0-a23e752f3026 not running after 3m0s; route 929d18bb-f5f6-48c9-8037-cd3dbb337b7a not running after 3m0s |

## fanout25

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 5000 | 40.9 | 73992 | 303.1 | sustained |

## fanout50

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
