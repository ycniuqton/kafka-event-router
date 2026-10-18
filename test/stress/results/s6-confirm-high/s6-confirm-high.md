# s6-confirm-high (local)

S6 confirmation, upper pass for fair-w2 / fair-w3 (no failing step in s6-confirm); otherwise as s6-confirm (5-min steps at the explore max and knee, 30 s warm-up) of s6-explore. Original text: S6 exploration pass — scale-out 1 -> 2 -> 3 workers, one route with replicas = workers, 48 partitions, 1 KB, maxInFlight 5000 (so each replica is CPU-bound, not in-flight-bound). Two CPU layouts: "fair" splits the same 6 cores 0-5 across the workers (0-5 | 0-2,3-5 | 0-1,2-3,4-5): total CPU constant, shows the cost of splitting; "c2" gives every worker its own 2 cores (0-1 | 0-1,2-3 | = fair-w3): CPU grows with workers, shows scaling. 1-min steps (20 s warm-up); s6-confirm re-checks the knees with 5-min steps. Partition ownership is snapshotted into results/s6-confirm-high/spread-<case>.txt. Local only (workers.sh).


Run `stress-s6-101717073953` on vps-c0215fee (14 CPUs), 2026-10-17T15:07:39-02:00 → 2026-10-17T15:20:47-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| fair-w2 | 1 | 0 | 115000 | lag_growing | 83886.1 | 0 | 0 | 1586 | 5.89 |  |
| fair-w3 | 1 | 0 | 90000 | lag_growing | 121634.8 | 0 | 0 | 5725 | 4.91 |  |

## fair-w2

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 115000 | 114993 | 4718.5 | 1600456 | 74448.9 | lag_growing |
| 1 | 130000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## fair-w3

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 90000 | 89970 | 9647.6 | 2607330 | 99614.7 | lag_growing |
| 1 | 100000 | 0 | 0.0 | 0 | 133169.2 | insufficient_samples |
