# stg-s3 (staging)

Staging S3 - route count 10 -> 50 -> 80 at 10 ev/s/route, incremental. Capped below the staging worker's default scheduler capacity (100 load units = ~89 one-partition routes next to orders-smoke).


Run `stress-stg-101719013263` on vps-157a7542 (3 CPUs), 2026-10-17T17:01:32-02:00 → 2026-10-17T17:08:27-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| r10 | 10 | 100 | 0 |  | 78.8 | 0 | 0 | 1614 | 0.09 |  |
| r50 | 50 | 500 | 0 |  | 165.9 | 0 | 0 | 2753 | 0.26 |  |
| r80 | 80 | 0 | 800 | p99_over_limit | 3113.0 | 0 | 0 | 2687 | 0.44 |  |

## r10

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 100 | 100 | -0.1 | 27 | 78.8 | sustained |

## r50

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 500 | 500 | -0.1 | 290 | 167.9 | sustained |

## r80

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 800 | 800 | -0.8 | 386 | 3113.0 | p99_over_limit |
