# s7-failure-under-load (local)

S7 — failures under load at ~70% of the S2 p12 knee (65k ev/s with the default maxInFlight 1000 -> 45k ev/s; the JS DLQ case runs at ~70% of the S5 "js" knee). Zero lost events required: every acknowledged event must reach the output or the DLQ (measure includes DLQ topics). Cases: kill -9 the worker and restart it 10 s later; graceful Kafka restart; kill -9 Kafka and restart it 10 s later; 50% of events fail into the DLQ (recipe convert_type error, and JS throw); exactly-once + kill -9 worker. Duplicates are allowed for at_least_once and reported. Partition ownership snapshots go to results/s7/spread-<case>.txt. Local only.


Run `stress-s7-101717212070` on vps-c0215fee (14 CPUs), 2026-10-17T15:21:20-02:00 → 2026-10-17T16:05:28-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| kill-worker | 1 | 0 | 45000 | lag_growing | 167772.2 | 0 | 6531 | 1434 | 4.18 |  |
| restart-kafka | 1 | 0 | 45000 | p99_over_limit | 11403.3 | 0 | 15318 | 752 | 3.65 |  |
| kill-kafka | 1 | 0 | 45000 | p99_over_limit | 23855.1 | 0 | 12585 | 950 | 3.70 |  |
| dlq-50pct-recipe | 1 | 45000 | 0 |  | 165.9 | 0 | 0 | 754 | 3.33 |  |
| dlq-50pct-js | 1 | 3500 | 0 |  | 704.5 | 0 | 0 | 661 | 4.25 |  |
| exactly-once-kill-worker | 1 | 0 | 45000 | lag_growing | 266338.3 | 0 | 0 | 794 | 1.47 |  |

## kill-worker

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 45000 | 44911 | 17082.0 | 4818294 | 136314.9 | lag_growing |

## restart-kafka

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 45000 | 44999 | -283.9 | 12649 | 11534.3 | p99_over_limit |

## kill-kafka

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 45000 | 45000 | -701.7 | 12826 | 23855.1 | p99_over_limit |

## dlq-50pct-recipe

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 45000 | 45000 | -1.9 | 14207 | 165.9 | sustained |

## dlq-50pct-js

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 3500 | 3500 | 0.6 | 765 | 704.5 | sustained |

## exactly-once-kill-worker

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 45000 | 44999 | 18396.1 | 6545559 | 180355.1 | lag_growing |
