# s5-explore-cpu (local)

S5 CPU-bound variant — same rule shapes as s5-explore but maxInFlight 5000 so the route is limited by worker CPU (S2 p12-mif5000: 80k ev/s) instead of the in-flight window. 1-min steps (20 s warm-up) from 50k.


Run `stress-s5-101714055097` on vps-c0215fee (14 CPUs), 2026-10-17T12:05:50-02:00 → 2026-10-17T12:29:42-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| baseline | 1 | 87500 | 95000 | lag_growing | 5898.2 | 0 | 0 | 687 | 5.06 |  |
| f50 | 1 | 72500 | 80000 | p99_over_limit | 548.9 | 0 | 0 | 717 | 5.05 |  |
| s5 | 1 | 72500 | 80000 | p99_over_limit | 3866.6 | 0 | 0 | 661 | 5.15 |  |
| s20 | 1 | 57500 | 65000 | lag_growing | 11927.6 | 0 | 0 | 719 | 5.35 |  |
| f50-s20 | 1 | 50000 | 57500 | lag_growing | 3702.8 | 0 | 0 | 789 | 5.44 |  |

## baseline

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 50000 | 49996 | -15.2 | 8371 | 34.8 | sustained |
| 1 | 57500 | 57496 | -5.8 | 9982 | 141.3 | sustained |
| 2 | 65000 | 64994 | 208.3 | 12448 | 778.2 | sustained |
| 3 | 72500 | 72492 | -17.2 | 12802 | 397.3 | sustained |
| 4 | 80000 | 79991 | -553.0 | 13960 | 688.1 | sustained |
| 5 | 87500 | 87489 | -71.7 | 18418 | 372.7 | sustained |
| 6 | 95000 | 94987 | 3468.8 | 189008 | 9568.3 | lag_growing |
| 7 | 105000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## f50

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 50000 | 49994 | 17.0 | 13166 | 30.0 | sustained |
| 1 | 57500 | 57496 | -2.6 | 13984 | 180.2 | sustained |
| 2 | 65000 | 64992 | -6.4 | 15723 | 190.5 | sustained |
| 3 | 72500 | 72495 | -180.0 | 18455 | 520.2 | sustained |
| 4 | 80000 | 79994 | 184.4 | 21906 | 1310.7 | p99_over_limit |
| 5 | 87500 | 22109 | 0.0 | 0 | 0.0 | insufficient_samples |

## s5

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 50000 | 49995 | -2.2 | 21265 | 46.1 | sustained |
| 1 | 57500 | 57494 | 2.2 | 24883 | 137.2 | sustained |
| 2 | 65000 | 64993 | 2.7 | 28151 | 417.8 | sustained |
| 3 | 72500 | 72495 | -182.9 | 32245 | 389.1 | sustained |
| 4 | 80000 | 79991 | 461.6 | 97762 | 4456.4 | p99_over_limit |
| 5 | 87500 | 9791 | 0.0 | 0 | 0.0 | insufficient_samples |

## s20

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 50000 | 49995 | -57.7 | 22390 | 139.3 | sustained |
| 1 | 57500 | 57494 | -5.8 | 27545 | 426.0 | sustained |
| 2 | 65000 | 64995 | 2621.6 | 226449 | 11010.0 | lag_growing |
| 3 | 36250 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## f50-s20

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 50000 | 49995 | -38.6 | 14759 | 331.8 | sustained |
| 1 | 57500 | 57495 | 757.6 | 48351 | 3768.3 | lag_growing |
| 2 | 32500 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
