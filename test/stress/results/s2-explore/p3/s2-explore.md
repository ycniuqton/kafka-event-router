# s2-explore (local)

S2 exploration pass — same routes as s2-single-route-ramp but 1-min steps (20 s warm-up) to locate the knee of each case quickly. The knee is then confirmed with 5-min steps around it (s2-single-route-ramp with a narrowed ramp).


Run `stress-s2-101707194816` on vps-c0215fee (14 CPUs), 2026-10-17T05:19:48-02:00 → 2026-10-17T05:28:19-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p3 | 1 | 50000 | 65000 | lag_growing | 9699.3 | 0 | 0 | 734 | 3.69 |  |

## p3

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 4999 | 1.0 | 1288 | 18.4 | sustained |
| 1 | 10000 | 9999 | -1.9 | 2410 | 18.9 | sustained |
| 2 | 15000 | 14999 | 1.8 | 3759 | 19.7 | sustained |
| 3 | 20000 | 19998 | -1.3 | 5021 | 20.0 | sustained |
| 4 | 30000 | 29998 | -9.6 | 7443 | 22.8 | sustained |
| 5 | 40000 | 39996 | -1.7 | 9840 | 74.8 | sustained |
| 6 | 50000 | 49995 | -108.4 | 12569 | 868.4 | sustained |
| 7 | 65000 | 64991 | 2659.1 | 195253 | 7143.4 | lag_growing |
| 8 | 80000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
