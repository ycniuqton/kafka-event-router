# wave2-mem (local)

Wave-2 memory overload: worker started with GOMEMLIMIT=512MiB and ROUTER_WORKER_MEMORY_BUDGET_BYTES=268435456 (256 MiB). 25 routes fan out one 12-partition input at 5k ev/s (output 125k ev/s, more than one worker can produce: the destinations are the slow side; in wave 1 this case grew RSS to 4.8 GB). Pass = no crash/OOM, RSS under 512 MiB, lost = 0, router_worker_memory_paused_routes > 0. Delivery tracked per output topic.


Run `stress-wave2-101816362253` on vps-c0215fee (14 CPUs), 2026-10-18T14:36:22-02:00 → 2026-10-18T14:49:18-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| fanout50 | 50 | 5000 | 0 |  | 578814.0 | 0 | 0 | 5243 | 5.57 |  |

## fanout50

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 5000 | 188281.0 | 33922943 | 148897.8 | sustained |
