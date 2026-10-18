# s2-explore (local)

S2 exploration pass — same routes as s2-single-route-ramp but 1-min steps (20 s warm-up) to locate the knee of each case quickly. The knee is then confirmed with 5-min steps around it (s2-single-route-ramp with a narrowed ramp).


Run `stress-s2-101707281911` on vps-c0215fee (14 CPUs), 2026-10-17T05:28:19-02:00 → 2026-10-17T05:36:36-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p12 | 1 | 50000 | 65000 | p99_over_limit | 606.2 | 0 | 0 | 941 | 4.30 |  |

## p12

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 4999 | 1.4 | 1729 | 19.5 | sustained |
| 1 | 10000 | 9999 | -1.1 | 3305 | 20.0 | sustained |
| 2 | 15000 | 14999 | 0.5 | 4992 | 21.0 | sustained |
| 3 | 20000 | 19998 | 0.2 | 6674 | 21.5 | sustained |
| 4 | 30000 | 29997 | -4.4 | 10156 | 39.9 | sustained |
| 5 | 40000 | 39996 | 0.7 | 13511 | 149.5 | sustained |
| 6 | 50000 | 49997 | 10.3 | 16864 | 149.5 | sustained |
| 7 | 65000 | 64997 | 264.4 | 30648 | 1359.9 | p99_over_limit |
| 8 | 40000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
