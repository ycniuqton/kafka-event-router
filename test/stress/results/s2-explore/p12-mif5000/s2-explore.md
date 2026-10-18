# s2-explore (local)

S2 exploration pass — same routes as s2-single-route-ramp but 1-min steps (20 s warm-up) to locate the knee of each case quickly. The knee is then confirmed with 5-min steps around it (s2-single-route-ramp with a narrowed ramp).


Run `stress-s2-101707471846` on vps-c0215fee (14 CPUs), 2026-10-17T05:47:18-02:00 → 2026-10-17T05:57:54-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p12-mif5000 | 1 | 80000 | 100000 | lag_growing | 21233.7 | 0 | 0 | 752 | 5.03 |  |

## p12-mif5000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 4999 | -1.0 | 1832 | 19.5 | sustained |
| 1 | 10000 | 9999 | 0.7 | 3728 | 20.0 | sustained |
| 2 | 15000 | 14999 | 0.6 | 5695 | 20.7 | sustained |
| 3 | 20000 | 19998 | 0.3 | 7514 | 21.2 | sustained |
| 4 | 30000 | 29997 | -3.6 | 11226 | 37.4 | sustained |
| 5 | 40000 | 39997 | 2.1 | 15128 | 42.5 | sustained |
| 6 | 50000 | 49997 | -4.3 | 19000 | 145.4 | sustained |
| 7 | 65000 | 64994 | -59.1 | 24469 | 114.7 | sustained |
| 8 | 80000 | 79994 | 521.4 | 46866 | 942.1 | sustained |
| 9 | 100000 | 99991 | 13808.4 | 897523 | 19136.5 | lag_growing |
| 10 | 130000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
