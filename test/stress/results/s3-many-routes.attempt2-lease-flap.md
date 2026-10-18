# s3-many-routes (local)

S3 — route count 10 -> 50 -> 100 -> 250 -> 500 -> 1000 at a low per-route rate (10 ev/s/route), incremental (routes of earlier cases stay deployed). Records per-route deploy->running latency, worker goroutines/RSS/CPU, producer clients, Kafka lag and e2e latency per step. The worker needs ROUTER_WORKER_CAPACITY >= 1000 (default 100): ROUTER_WORKER_CAPACITY=1200 deploy/dev/run.sh restart worker. Scheduler cycle and PG connections are read from the api/worker logs and pg_stat_activity (see README) — not sampled by the runner.


Run `stress-s3-101710264304` on vps-c0215fee (14 CPUs), 2026-10-17T08:26:43-02:00 → 2026-10-17T09:06:18-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| r10 | 10 | 100 | 0 |  | 18.7 | 0 | 0 | 1179 | 0.10 |  |
| r50 | 50 | 500 | 0 |  | 18.9 | 0 | 0 | 2228 | 0.26 |  |
| r100 | 100 | 1000 | 0 |  | 19.7 | 0 | 0 | 3277 | 0.45 |  |
| r250 | 250 | 2500 | 0 |  | 23.3 | 0 | 0 | 8323 | 0.93 |  |
| r500 | 500 | 5000 | 0 |  | 41.0 | 0 | 0 | 13500 | 1.71 |  |
| r1000 | 0 | 0 | 0 |  | 0.0 | 0 | 0 | 0 | 0.00 | 3/500 routes failed: route 563ba5c8-374f-4b17-b0c0-824fe5f37f68 not running after 3m0s; context canceled; context canceled |

## r10

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 100 | 100 | -0.0 | 72 | 18.7 | sustained |

## r50

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 500 | 500 | 0.0 | 248 | 18.9 | sustained |

## r100

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 1000 | 1000 | 0.0 | 547 | 19.7 | sustained |

## r250

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 2500 | 2500 | -0.1 | 1353 | 23.3 | sustained |

## r500

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 5000 | 0.1 | 2386 | 41.0 | sustained |

## r1000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
