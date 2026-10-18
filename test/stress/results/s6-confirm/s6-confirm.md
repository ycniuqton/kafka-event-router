# s6-confirm (local)

S6 confirmation pass (5-min steps at the explore max and knee, 30 s warm-up) of s6-explore. Original text: S6 exploration pass — scale-out 1 -> 2 -> 3 workers, one route with replicas = workers, 48 partitions, 1 KB, maxInFlight 5000 (so each replica is CPU-bound, not in-flight-bound). Two CPU layouts: "fair" splits the same 6 cores 0-5 across the workers (0-5 | 0-2,3-5 | 0-1,2-3,4-5): total CPU constant, shows the cost of splitting; "c2" gives every worker its own 2 cores (0-1 | 0-1,2-3 | = fair-w3): CPU grows with workers, shows scaling. 1-min steps (20 s warm-up); s6-confirm re-checks the knees with 5-min steps. Partition ownership is snapshotted into results/s6-confirm/spread-<case>.txt. Local only (workers.sh).


Run `stress-s6-101716133712` on vps-c0215fee (14 CPUs), 2026-10-17T14:13:37-02:00 → 2026-10-17T15:07:27-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| fair-w1 | 1 | 90000 | 100000 | lag_growing | 46137.3 | 0 | 0 | 961 | 5.37 |  |
| fair-w2 | 1 | 100000 | 0 |  | 458.8 | 0 | 0 | 923 | 5.77 |  |
| fair-w3 | 1 | 80000 | 0 |  | 614.4 | 0 | 0 | 986 | 4.47 |  |
| c2-w1 | 1 | 40000 | 50000 | lag_growing | 58720.3 | 0 | 0 | 966 | 2.01 |  |
| c2-w2 | 1 | 70000 | 80000 | lag_growing | 125829.1 | 0 | 0 | 939 | 4.01 |  |

## fair-w1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 90000 | 89998 | -2.3 | 38358 | 860.2 | sustained |
| 1 | 100000 | 99998 | 3043.1 | 967683 | 45088.8 | lag_growing |

## fair-w2

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 90000 | 89988 | 16.7 | 32454 | 364.5 | sustained |
| 1 | 100000 | 99997 | 6.5 | 31543 | 516.1 | sustained |

## fair-w3

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 70000 | 69999 | -2.6 | 18124 | 323.6 | sustained |
| 1 | 80000 | 79998 | 1.8 | 25857 | 720.9 | sustained |

## c2-w1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 40000 | 39999 | -1.7 | 20491 | 434.2 | sustained |
| 1 | 50000 | 49999 | 1989.9 | 614114 | 61866.0 | lag_growing |

## c2-w2

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 70000 | 69944 | 10.7 | 42296 | 389.1 | sustained |
| 1 | 80000 | 79204 | 9116.7 | 4001422 | 73400.3 | lag_growing |
