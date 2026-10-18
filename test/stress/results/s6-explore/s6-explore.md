# s6-explore (local)

S6 exploration pass — scale-out 1 -> 2 -> 3 workers, one route with replicas = workers, 48 partitions, 1 KB, maxInFlight 5000 (so each replica is CPU-bound, not in-flight-bound). Two CPU layouts: "fair" splits the same 6 cores 0-5 across the workers (0-5 | 0-2,3-5 | 0-1,2-3,4-5): total CPU constant, shows the cost of splitting; "c2" gives every worker its own 2 cores (0-1 | 0-1,2-3 | = fair-w3): CPU grows with workers, shows scaling. 1-min steps (20 s warm-up); s6-confirm re-checks the knees with 5-min steps. Partition ownership is snapshotted into results/s6-explore/spread-<case>.txt. Local only (workers.sh).


Run `stress-s6-101715301427` on vps-c0215fee (14 CPUs), 2026-10-17T13:30:14-02:00 → 2026-10-17T14:12:53-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| fair-w1 | 1 | 90000 | 100000 | lag_growing | 14811.1 | 0 | 0 | 979 | 5.29 |  |
| fair-w2 | 1 | 90000 | 100000 | p99_over_limit | 557.1 | 0 | 0 | 974 | 5.71 |  |
| fair-w3 | 1 | 70000 | 80000 | p99_over_limit | 843.8 | 0 | 0 | 755 | 4.46 |  |
| c2-w1 | 1 | 40000 | 50000 | lag_growing | 11927.6 | 0 | 0 | 2151 | 2.02 |  |
| c2-w2 | 1 | 70000 | 80000 | p99_over_limit | 655.4 | 0 | 0 | 948 | 3.98 |  |

## fair-w1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -1.1 | 2904 | 28.4 | sustained |
| 1 | 20000 | 19999 | 13.1 | 5979 | 59.9 | sustained |
| 2 | 30000 | 29997 | 0.2 | 9051 | 65.5 | sustained |
| 3 | 40000 | 39996 | -5.5 | 11856 | 99.3 | sustained |
| 4 | 50000 | 49994 | -17.4 | 15028 | 120.8 | sustained |
| 5 | 60000 | 59995 | 3.1 | 18291 | 139.3 | sustained |
| 6 | 70000 | 69993 | 29.8 | 21520 | 217.1 | sustained |
| 7 | 80000 | 79993 | 37.4 | 25342 | 239.6 | sustained |
| 8 | 90000 | 89989 | 259.0 | 38660 | 639.0 | sustained |
| 9 | 100000 | 99982 | 8142.3 | 476530 | 17301.5 | lag_growing |
| 10 | 115000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## fair-w2

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 10000 | 1.2 | 2867 | 34.8 | sustained |
| 1 | 20000 | 19998 | 1.2 | 5729 | 75.8 | sustained |
| 2 | 30000 | 29997 | 0.9 | 8702 | 100.4 | sustained |
| 3 | 40000 | 39997 | 1.6 | 11528 | 80.9 | sustained |
| 4 | 50000 | 49994 | 0.9 | 14354 | 82.9 | sustained |
| 5 | 60000 | 59994 | 0.2 | 17525 | 91.1 | sustained |
| 6 | 70000 | 69995 | -33.9 | 20642 | 133.1 | sustained |
| 7 | 80000 | 79945 | -167.1 | 22540 | 172.0 | sustained |
| 8 | 90000 | 89986 | -104.3 | 34140 | 421.9 | sustained |
| 9 | 100000 | 99987 | -98.2 | 46108 | 1179.6 | p99_over_limit |
| 10 | 115000 | 54116 | 0.0 | 0 | 0.0 | insufficient_samples |

## fair-w3

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | 0.5 | 3034 | 50.2 | sustained |
| 1 | 20000 | 19998 | 19.0 | 5933 | 54.3 | sustained |
| 2 | 30000 | 29997 | -4.3 | 8915 | 74.8 | sustained |
| 3 | 40000 | 39996 | -2.7 | 12117 | 88.1 | sustained |
| 4 | 50000 | 49939 | -156.0 | 12184 | 100.4 | sustained |
| 5 | 60000 | 59989 | -48.9 | 18466 | 161.8 | sustained |
| 6 | 70000 | 69991 | 16.4 | 26903 | 286.7 | sustained |
| 7 | 80000 | 79988 | 358.0 | 50160 | 1441.8 | p99_over_limit |
| 8 | 90000 | 81124 | 0.0 | 0 | 0.0 | insufficient_samples |

## c2-w1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -0.7 | 3299 | 31.2 | sustained |
| 1 | 20000 | 19998 | 1.8 | 6601 | 68.6 | sustained |
| 2 | 30000 | 29998 | 6.6 | 10483 | 133.1 | sustained |
| 3 | 40000 | 39997 | -55.5 | 18707 | 495.6 | sustained |
| 4 | 50000 | 49995 | 3063.4 | 209089 | 13500.4 | lag_growing |
| 5 | 60000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## c2-w2

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | 0.0 | 2406 | 44.0 | sustained |
| 1 | 20000 | 19998 | 0.5 | 4687 | 39.9 | sustained |
| 2 | 30000 | 29998 | -3.0 | 7078 | 92.2 | sustained |
| 3 | 40000 | 39996 | 1.2 | 9914 | 124.9 | sustained |
| 4 | 50000 | 49995 | 10.0 | 12334 | 106.5 | sustained |
| 5 | 60000 | 59995 | -15.5 | 15502 | 188.4 | sustained |
| 6 | 70000 | 69994 | -194.2 | 18809 | 266.2 | sustained |
| 7 | 80000 | 79991 | -293.6 | 40027 | 1146.9 | p99_over_limit |
| 8 | 90000 | 84824 | 0.0 | 0 | 794.6 | insufficient_samples |
