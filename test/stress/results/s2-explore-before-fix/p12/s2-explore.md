# s2-explore (local)

S2 exploration pass — same routes as s2-single-route-ramp but 1-min steps (20 s warm-up) to locate the knee of each case quickly. The knee is then confirmed with 5-min steps around it (s2-single-route-ramp with a narrowed ramp).


Run `stress-s2-101706533672` on vps-c0215fee (14 CPUs), 2026-10-17T04:53:36-02:00 → 2026-10-17T05:07:43-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p12 | 1 | 65000 | 80000 | lag_growing | 0.0 | 0 | 0 | 803 | 4.26 |  |

## p12

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 4999 | -0.2 | 1480 | 19.5 | sustained |
| 1 | 10000 | 9999 | 0.1 | 2889 | 20.2 | sustained |
| 2 | 15000 | 14999 | 3.3 | 4502 | 20.5 | sustained |
| 3 | 20000 | 19998 | -1.0 | 5793 | 21.2 | sustained |
| 4 | 30000 | 29996 | -0.8 | 8892 | 24.6 | sustained |
| 5 | 40000 | 39997 | 0.1 | 12072 | 89.1 | sustained |
| 6 | 50000 | 49996 | 0.8 | 14841 | 151.6 | sustained |
| 7 | 65000 | 64994 | -88.0 | 19380 | 303.1 | sustained |
| 8 | 80000 | 79992 | 11981.1 | 732844 | 18874.4 | lag_growing |
| 9 | 50000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
