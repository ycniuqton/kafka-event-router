# s2-confirm (local)

S2 confirmation pass — 5-min steps around the knee found by s2-explore (1-min steps), with the full criteria (lag slope <= max(50, 1% of rate)/s after a 30 s warm-up for the whole step AND p99 end-to-end < 1 s). One route, 1 KB JSON (10 KB for p12-10kb), no filter/transform.


Run `stress-s2-101712112300` on vps-c0215fee (14 CPUs), 2026-10-17T10:11:23-02:00 → 2026-10-17T10:47:23-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p1 | 1 | 30000 | 35000 | lag_growing | 37224.4 | 0 | 0 | 674 | 1.80 |  |
| p12 | 1 | 65000 | 0 |  | 206.8 | 0 | 0 | 660 | 4.04 |  |

## p1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 25000 | 25000 | -1.1 | 5908 | 109.6 | sustained |
| 1 | 30000 | 30000 | -10.1 | 7630 | 471.0 | sustained |
| 2 | 35000 | 34999 | 4096.0 | 1224715 | 34777.3 | lag_growing |

## p12

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 40000 | 39999 | 0.7 | 7361 | 47.6 | sustained |
| 1 | 50000 | 49999 | 1.1 | 8911 | 172.0 | sustained |
| 2 | 57500 | 57499 | 0.8 | 10759 | 202.8 | sustained |
| 3 | 65000 | 64999 | -2.3 | 12483 | 290.8 | sustained |
