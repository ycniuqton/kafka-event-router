# s3-many-routes (local)

S3 — route count 10 -> 50 -> 100 -> 250 -> 500 -> 1000 at a low per-route rate (10 ev/s/route), incremental (routes of earlier cases stay deployed). Records per-route deploy->running latency, worker goroutines/RSS/CPU, producer clients, Kafka lag and e2e latency per step. The worker needs ROUTER_WORKER_CAPACITY >= 1000 (default 100): ROUTER_WORKER_CAPACITY=1200 deploy/dev/run.sh restart worker. Scheduler cycle and PG connections are read from the api/worker logs and pg_stat_activity (see README) — not sampled by the runner.


Run `stress-s3-101710130355` on vps-c0215fee (14 CPUs), 2026-10-17T08:13:03-02:00 → 2026-10-17T08:24:11-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| r10 | 10 | 100 | 0 |  | 18.7 | 0 | 0 | 1440 | 0.11 |  |
| r50 | 50 | 500 | 0 |  | 18.9 | 0 | 0 | 2015 | 0.27 |  |
| r100 | 100 | 1000 | 0 |  | 19.7 | 0 | 0 | 3178 | 0.45 |  |
| r250 | 0 | 0 | 0 |  | 0.0 | 0 | 0 | 0 | 0.00 | 95/150 routes failed: deploy: HTTP 429 {"type":"https://eventrouter.dev/problems/rate-limited","title":"Too Many Requests","status":429,"detail":"Too many requests. Slow down and retry after the time in Retry-After.","instance":"/api/v1/routes/49e9a5f2-8af5-40cd-b5fd-a51b56db765a/deploy","request_id":"01M54P5NRF7V2BP2A11RC9A9AC","retry_after":1}
; create route stress-s3-101710130355-r250-r156: HTTP 429 {"type":"https://eventrouter.dev/problems/rate-limited","title":"Too Many Requests","status":429,"detail":"Too many requests. Slow down and retry after the time in Retry-After.","instance":"/api/v1/routes","request_id":"01M54P5NSC5KT4H25S4RTEHMZP","retry_after":1}
; create route stress-s3-101710130355-r250-r157: HTTP 429 {"type":"https://eventrouter.dev/problems/rate-limited","title":"Too Many Requests","status":429,"detail":"Too many requests. Slow down and retry after the time in Retry-After.","instance":"/api/v1/routes","request_id":"01M54P5NTRDJ7RR7FBF7PPBK03","retry_after":1}
 |
| r500 | 0 | 0 | 0 |  | 0.0 | 0 | 0 | 0 | 0.00 | 250/250 routes failed: deploy: HTTP 429 {"type":"https://eventrouter.dev/problems/rate-limited","title":"Too Many Requests","status":429,"detail":"Too many requests. Slow down and retry after the time in Retry-After.","instance":"/api/v1/routes/efdeafa7-7dc7-4064-bcd1-65aa214cf879/deploy","request_id":"01M54P5RVXAZEJ3SEASN4WFFTW","retry_after":1}
; deploy: HTTP 429 {"type":"https://eventrouter.dev/problems/rate-limited","title":"Too Many Requests","status":429,"detail":"Too many requests. Slow down and retry after the time in Retry-After.","instance":"/api/v1/routes/3361f7f7-e9cd-458f-9075-6045fec507c4/deploy","request_id":"01M54P5RW3DSD5ZXGTWCX1KW9N","retry_after":1}
; deploy: HTTP 429 {"type":"https://eventrouter.dev/problems/rate-limited","title":"Too Many Requests","status":429,"detail":"Too many requests. Slow down and retry after the time in Retry-After.","instance":"/api/v1/routes/300d1f1a-05bf-4d08-96cd-0ce7954dacbf/deploy","request_id":"01M54P5RW7KNCP400245Q4XG0H","retry_after":1}
 |
| r1000 | 0 | 0 | 0 |  | 0.0 | 0 | 0 | 0 | 0.00 | 500/500 routes failed: create route stress-s3-101710130355-r1000-r500: HTTP 429 {"type":"https://eventrouter.dev/problems/rate-limited","title":"Too Many Requests","status":429,"detail":"Too many requests. Slow down and retry after the time in Retry-After.","instance":"/api/v1/routes","request_id":"01M54P5SFA2GS6PKHRQP2D2E02","retry_after":1}
; create route stress-s3-101710130355-r1000-r501: HTTP 429 {"type":"https://eventrouter.dev/problems/rate-limited","title":"Too Many Requests","status":429,"detail":"Too many requests. Slow down and retry after the time in Retry-After.","instance":"/api/v1/routes","request_id":"01M54P5SFA2GS6PKHRQE1RMCQH","retry_after":1}
; create route stress-s3-101710130355-r1000-r502: HTTP 429 {"type":"https://eventrouter.dev/problems/rate-limited","title":"Too Many Requests","status":429,"detail":"Too many requests. Slow down and retry after the time in Retry-After.","instance":"/api/v1/routes","request_id":"01M54P5SFA2GS6PKHRQGSMS4PW","retry_after":1}
 |

## r10

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 100 | 100 | 0.0 | 82 | 18.7 | sustained |

## r50

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 500 | 500 | 0.0 | 310 | 18.9 | sustained |

## r100

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 1000 | 1000 | -0.0 | 622 | 19.7 | sustained |

## r250

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|

## r500

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|

## r1000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|

Cleanup errors: delete connection 2c31f289-532b-43e7-ab0e-1465da51d241: HTTP 409 {"type":"https://eventrouter.dev/problems/connection-in-use","title":"Conflict","status":409,"detail":"This connection is used by routes. Point them to another connection or archive them, then delete again.","instance":"/api/v1/connections/2c31f289-532b-43e7-ab0e-1465da51d241","request_id":"01M54P7YJ0V3RWRCWHDM3SSZKV","routes":[{"id":"53e3ac48-d065-4763-94d4-4c20bc3ab884","name":"stress-s3-101710130355-r10-r0"},{"id":"18cd1b73-7b8f-40dd-9fd0-8882b2ff83d0","name":"stress-s3-101710130355-r10-r1"},{"id":"ab456efc-42cb-4fe4-9263-c06c19668e41","name":"stress-s3-101710130355-r10-r3"},{"id":"b7826032
