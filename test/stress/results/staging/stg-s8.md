# stg-s8 (staging)

Staging 40-minute soak at 70% of the staging S2 p12 knee (7000), one route, 12 partitions, 1 KB, default maxInFlight, 15 s samples. Watch RSS / goroutines for growth, lag and p99 drift, loss; host side in watch.csv.


Run `stress-stg-101719093631` on vps-157a7542 (3 CPUs), 2026-10-17T17:09:36-02:00 → 2026-10-17T17:49:58-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| soak-40m | 1 | 7000 | 0 |  | 925.7 | 0 | 0 | 834 | 0.72 |  |

## soak-40m

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 7000 | 6996 | -0.2 | 174 | 761.9 | sustained |
