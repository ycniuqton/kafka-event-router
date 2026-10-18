# s2-explore (local)

S2 exploration pass — same routes as s2-single-route-ramp but 1-min steps (20 s warm-up) to locate the knee of each case quickly. The knee is then confirmed with 5-min steps around it (s2-single-route-ramp with a narrowed ramp).


Run `stress-s2-101707363633` on vps-c0215fee (14 CPUs), 2026-10-17T05:36:36-02:00 → 2026-10-17T05:44:51-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p48 | 1 | 50000 | 65000 | lag_growing | 2916.4 | 0 | 0 | 2672 | 4.08 |  |

## p48

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 5000 | -0.4 | 1628 | 22.8 | sustained |
| 1 | 10000 | 9999 | -2.0 | 3376 | 29.7 | sustained |
| 2 | 15000 | 14999 | 2.1 | 5111 | 36.9 | sustained |
| 3 | 20000 | 19998 | -1.0 | 6785 | 89.1 | sustained |
| 4 | 30000 | 29997 | -5.1 | 10045 | 129.0 | sustained |
| 5 | 40000 | 39997 | -6.8 | 13686 | 89.1 | sustained |
| 6 | 50000 | 49994 | -15.9 | 17102 | 184.3 | sustained |
| 7 | 65000 | 64988 | 1343.3 | 73171 | 5832.7 | lag_growing |
| 8 | 40000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
