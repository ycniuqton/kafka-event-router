# s2-explore (local)

S2 exploration pass — same routes as s2-single-route-ramp but 1-min steps (20 s warm-up) to locate the knee of each case quickly. The knee is then confirmed with 5-min steps around it (s2-single-route-ramp with a narrowed ramp).


Run `stress-s2-101706402993` on vps-c0215fee (14 CPUs), 2026-10-17T04:40:29-02:00 → 2026-10-17T04:53:36-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p3 | 1 | 50000 | 65000 | lag_growing | 0.0 | 0 | 0 | 633 | 3.78 |  |

## p3

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 5000 | -0.6 | 2214 | 18.7 | sustained |
| 1 | 10000 | 9999 | 0.5 | 4429 | 18.9 | sustained |
| 2 | 15000 | 14999 | 12.6 | 7320 | 19.5 | sustained |
| 3 | 20000 | 19998 | 0.6 | 8965 | 21.2 | sustained |
| 4 | 30000 | 29997 | -0.4 | 13398 | 145.4 | sustained |
| 5 | 40000 | 39999 | 32.3 | 17879 | 90.1 | sustained |
| 6 | 50000 | 49996 | -0.4 | 22613 | 153.6 | sustained |
| 7 | 65000 | 64991 | 4650.7 | 313254 | 9830.4 | lag_growing |
| 8 | 40000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
