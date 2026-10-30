# wave2-s7 (local)

Wave-2 S7 quick: kill -9 + restart of the worker at 45k ev/s, and 50 % of events into the DLQ (recipe error) at 45k ev/s. Zero lost required (measure includes the DLQ). Fast codec, default auto in-flight.


Run `stress-wave2-101814153528` on vps-c0215fee (14 CPUs), 2026-10-18T12:15:35-02:00 → 2026-10-18T12:24:02-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| kill-worker | 1 | 0 | 45000 | p99_over_limit | 44564.5 | 0 | 14210 | 801 | 3.87 |  |
| dlq-50pct-recipe | 1 | 45000 | 0 |  | 92.2 | 0 | 0 | 725 | 3.28 |  |

## kill-worker

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 45000 | 44999 | -764.7 | 38253 | 44564.5 | p99_over_limit |

## dlq-50pct-recipe

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 45000 | 44999 | -1.2 | 21686 | 92.2 | sustained |
