# s2-explore (local)

S2 exploration pass — same routes as s2-single-route-ramp but 1-min steps (20 s warm-up) to locate the knee of each case quickly. The knee is then confirmed with 5-min steps around it (s2-single-route-ramp with a narrowed ramp).


Run `stress-s2-101707445111` on vps-c0215fee (14 CPUs), 2026-10-17T05:44:51-02:00 → 2026-10-17T05:47:18-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p12-mif100 | 1 | 5000 | 10000 | lag_growing | 20447.2 | 0 | 0 | 824 | 0.77 |  |

## p12-mif100

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 4999 | -0.5 | 1387 | 19.5 | sustained |
| 1 | 10000 | 9999 | 1453.5 | 90506 | 19136.5 | lag_growing |
| 2 | 7500 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
