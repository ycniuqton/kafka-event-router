# s7-probe (local)

S7 follow-up — repeats kill-worker with a host I/O probe (dirty page cache, iowait, blocked tasks every 5 s) and partition-ownership snapshots every 20 s, to explain the slow, erratic catch-up seen in s7 kill-worker at 45k. Second case at 30k ev/s (~46% of the 65k knee) shows recovery with more headroom. Local only.


Run `stress-s7-101718054174` on vps-c0215fee (14 CPUs), 2026-10-17T16:05:41-02:00 → 2026-10-17T16:18:05-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| kill-worker-45k | 1 | 0 | 45000 | p99_over_limit | 44564.5 | 0 | 15421 | 850 | 4.41 |  |
| kill-worker-30k | 1 | 0 | 30000 | p99_over_limit | 49283.1 | 0 | 9966 | 1997 | 3.76 |  |

## kill-worker-45k

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 45000 | 44999 | -2099.0 | 28851 | 44564.5 | p99_over_limit |

## kill-worker-30k

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 30000 | 29999 | -1438.2 | 26407 | 49283.1 | p99_over_limit |
