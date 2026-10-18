# s2-confirm (local)

S2 confirmation pass — 5-min steps around the knee found by s2-explore (1-min steps), with the full criteria (lag slope <= max(50, 1% of rate)/s after a 30 s warm-up for the whole step AND p99 end-to-end < 1 s). One route, 1 KB JSON (10 KB for p12-10kb), no filter/transform.


Run `stress-s2-101708103236` on vps-c0215fee (14 CPUs), 2026-10-17T06:10:32-02:00 → 2026-10-17T08:12:46-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p1 | 1 | 30000 | 35000 | lag_growing | 30933.0 | 0 | 0 | 761 | 1.79 |  |
| p3 | 1 | 57500 | 0 |  | 335.9 | 0 | 0 | 806 | 3.56 |  |
| p12 | 1 | 65000 | 0 |  | 243.7 | 0 | 0 | 744 | 4.25 |  |
| p48 | 1 | 57500 | 0 |  | 231.4 | 0 | 0 | 1488 | 4.16 |  |
| p12-mif100 | 1 | 8000 | 0 |  | 33.3 | 0 | 0 | 969 | 0.80 |  |
| p12-mif5000 | 1 | 80000 | 90000 | p99_over_limit | 2015.2 | 0 | 0 | 735 | 5.03 |  |
| p12-10kb | 1 | 20000 | 0 |  | 512.0 | 0 | 0 | 733 | 3.91 |  |

## p1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 25000 | 25000 | 0.0 | 7459 | 107.5 | sustained |
| 1 | 30000 | 30000 | -0.5 | 9273 | 266.2 | sustained |
| 2 | 35000 | 35000 | 3411.3 | 1008945 | 28311.6 | lag_growing |

## p3

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 40000 | 39999 | -0.1 | 19294 | 170.0 | sustained |
| 1 | 50000 | 49999 | -0.5 | 23992 | 122.9 | sustained |
| 2 | 57500 | 57499 | -11.5 | 29125 | 507.9 | sustained |

## p12

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 40000 | 39999 | -1.4 | 11449 | 85.0 | sustained |
| 1 | 50000 | 49999 | -2.4 | 14518 | 108.5 | sustained |
| 2 | 57500 | 57499 | -5.1 | 16504 | 139.3 | sustained |
| 3 | 65000 | 64999 | -0.2 | 19104 | 462.8 | sustained |

## p48

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 40000 | 39999 | 1.1 | 16227 | 112.6 | sustained |
| 1 | 50000 | 49988 | -1.7 | 14539 | 213.0 | sustained |
| 2 | 57500 | 57499 | 0.1 | 22066 | 303.1 | sustained |

## p12-mif100

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 5000 | -0.0 | 1718 | 19.7 | sustained |
| 1 | 6500 | 6500 | -0.1 | 2181 | 22.3 | sustained |
| 2 | 8000 | 8000 | 0.3 | 2746 | 59.4 | sustained |

## p12-mif5000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 65000 | 64999 | 4.9 | 32127 | 192.5 | sustained |
| 1 | 72500 | 72498 | 0.3 | 36544 | 198.7 | sustained |
| 2 | 80000 | 79998 | -1.4 | 50081 | 245.8 | sustained |
| 3 | 90000 | 89998 | -29.1 | 91088 | 3113.0 | p99_over_limit |

## p12-10kb

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 12000 | 12000 | 0.3 | 3461 | 208.9 | sustained |
| 1 | 15000 | 15000 | -0.3 | 4387 | 319.5 | sustained |
| 2 | 17500 | 17500 | 5.8 | 16781 | 520.2 | sustained |
| 3 | 20000 | 20000 | -5.7 | 5932 | 753.7 | sustained |
