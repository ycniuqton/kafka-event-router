# s2-explore (local)

S2 exploration pass — same routes as s2-single-route-ramp but 1-min steps (20 s warm-up) to locate the knee of each case quickly. The knee is then confirmed with 5-min steps around it (s2-single-route-ramp with a narrowed ramp).


Run `stress-s2-101707575475` on vps-c0215fee (14 CPUs), 2026-10-17T05:57:54-02:00 → 2026-10-17T06:08:22-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p12-10kb | 1 | 20000 | 25000 | lag_growing | 12714.0 | 0 | 0 | 796 | 4.32 |  |

## p12-10kb

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 1000 | 1000 | -0.1 | 352 | 19.5 | sustained |
| 1 | 2000 | 2000 | 0.1 | 720 | 21.2 | sustained |
| 2 | 4000 | 3999 | -0.2 | 1429 | 27.9 | sustained |
| 3 | 6000 | 5999 | 1.3 | 2140 | 76.8 | sustained |
| 4 | 8000 | 7999 | -20.5 | 2829 | 72.7 | sustained |
| 5 | 10000 | 9999 | 1.4 | 3500 | 704.5 | sustained |
| 6 | 12000 | 11998 | -1.0 | 4231 | 243.7 | sustained |
| 7 | 15000 | 14998 | 43.5 | 5342 | 200.7 | sustained |
| 8 | 20000 | 19997 | -20.5 | 7138 | 868.4 | sustained |
| 9 | 25000 | 24996 | 606.3 | 89670 | 12845.1 | lag_growing |
