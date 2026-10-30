# wave2-s3 (local)

Wave-2 S3 quick pass: 100 -> 500 -> 1000 routes (incremental) at 10 ev/s/route, 2-min steps. Worker restarted with ROUTER_WORKER_CAPACITY=1200 (fast codec, default auto in-flight, memory budget on).


Run `stress-wave2-101813424829` on vps-c0215fee (14 CPUs), 2026-10-18T11:42:48-02:00 → 2026-10-18T12:10:34-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| r100 | 100 | 1000 | 0 |  | 20.2 | 0 | 0 | 5374 | 0.39 |  |
| r500 | 500 | 5000 | 0 |  | 38.9 | 0 | 0 | 15073 | 1.38 |  |
| r1000 | 1000 | 10000 | 0 |  | 307.2 | 0 | 0 | 48234 | 2.75 |  |

## r100

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 1000 | 1000 | 0.0 | 482 | 20.2 | sustained |

## r500

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 5000 | -0.3 | 2642 | 38.9 | sustained |

## r1000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -0.9 | 5187 | 307.2 | sustained |
