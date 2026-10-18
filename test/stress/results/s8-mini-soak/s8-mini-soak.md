# s8-mini-soak (local)

Local 20-minute mini-soak at 70% of the S2 p12 knee (65k -> 45k ev/s), one route, 12 partitions, 1 KB, default maxInFlight. Watch RSS / goroutines / heap for growth, lag and p99 drift, and loss. The 8 h version is s8-soak.


Run `stress-s8-101718244223` on vps-c0215fee (14 CPUs), 2026-10-17T16:24:42-02:00 → 2026-10-17T16:44:59-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| soak-20m | 1 | 0 | 45000 | p99_over_limit | 78643.2 | 0 | 0 | 2229 | 4.42 |  |

## soak-20m

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 45000 | 45000 | -1402.7 | 12168 | 78643.2 | p99_over_limit |
