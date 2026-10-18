# s2-explore (local)

S2 exploration pass — same routes as s2-single-route-ramp but 1-min steps (20 s warm-up) to locate the knee of each case quickly. The knee is then confirmed with 5-min steps around it (s2-single-route-ramp with a narrowed ramp).


Run `stress-s2-101706351246` on vps-c0215fee (14 CPUs), 2026-10-17T04:35:12-02:00 → 2026-10-17T04:40:29-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p1 | 1 | 20000 | 30000 | lag_growing | 1998.8 | 0 | 0 | 661 | 1.72 |  |

## p1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 4999 | -0.7 | 2169 | 18.4 | sustained |
| 1 | 10000 | 9999 | 0.1 | 4363 | 18.9 | sustained |
| 2 | 15000 | 14999 | 0.4 | 6546 | 22.3 | sustained |
| 3 | 20000 | 19999 | 2.8 | 8672 | 290.8 | sustained |
| 4 | 30000 | 29998 | 1211.9 | 73018 | 2015.2 | lag_growing |
| 5 | 20000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
