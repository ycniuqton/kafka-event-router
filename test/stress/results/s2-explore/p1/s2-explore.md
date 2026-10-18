# s2-explore (local)

S2 exploration pass — same routes as s2-single-route-ramp but 1-min steps (20 s warm-up) to locate the knee of each case quickly. The knee is then confirmed with 5-min steps around it (s2-single-route-ramp with a narrowed ramp).


Run `stress-s2-101707131885` on vps-c0215fee (14 CPUs), 2026-10-17T05:13:18-02:00 → 2026-10-17T05:19:48-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p1 | 1 | 30000 | 40000 | lag_growing | 18350.1 | 0 | 0 | 719 | 1.78 |  |

## p1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 5000 | 0.8 | 2113 | 18.4 | sustained |
| 1 | 10000 | 9999 | -0.6 | 4231 | 18.9 | sustained |
| 2 | 15000 | 15000 | -2.1 | 6374 | 19.5 | sustained |
| 3 | 20000 | 19998 | -1.4 | 8546 | 28.7 | sustained |
| 4 | 30000 | 29998 | -105.2 | 13250 | 260.1 | sustained |
| 5 | 40000 | 39995 | 9715.5 | 591421 | 14417.9 | lag_growing |
| 6 | 25000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
