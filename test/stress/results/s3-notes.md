# S3 many routes: notes

Final run: `s3-many-routes.json` / `.md` (attempt 3). The worker was pinned to cores 0-5 with `ROUTER_WORKER_CAPACITY=1200`
and the heartbeat fix (see below) applied. Each case is incremental, adds 1-partition routes at 10 ev/s/route, and runs 3 min at the total
rate. Extra 15 s samples (Kafka connections, fds, PG connections) are in `logs/s3-extra-samples.log`.

| routes | ev/s | sustained | p50 / p99 ms | lost | worker CPU | Kafka CPU | RSS MB | goroutines | worker→Kafka conns | PG conns (router_dev) | deploy→running p50 / p95 / max s | setup s |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| 10 | 100 | yes | 18 / 19 | 0 | 0.08 | 0.11 | 67 | 177 | 53 | 18 | 1.2 / 1.3 / 1.3 | 1 |
| 50 | 500 | yes | 13 / 19 | 0 | 0.23 | 0.28 | 83 | 699 | 232 | 18 | 1.7 / 2.4 / 2.4 | 6 |
| 100 | 1000 | yes | 13 / 20 | 0 | 0.38 | 0.45 | 102 | 1349 | 402 | 18 | 1.9 / 3.7 / 3.8 | 10 |
| 250 | 2500 | yes | 15 / 23 | 0 | 0.79 | 0.88 | 154 | 3302 | 1052 | 18 | 3.3 / 8.3 / 9.8 | 57 |
| 500 | 5000 | yes | 19 / 39 | 0 | 1.53 | 1.61 | 235 | 6551 | 1645 | 18 | 5.8 / 13.9 / 21.8 | 154 |
| 1000 | 10000 | yes | 88 / 244 | 0 | 3.00 | 2.76 | 416 | 13073 | 3042 | 18 | 10.5 / 32.5 / 74.7 | 649 |

Per route: ~13 goroutines, ~3 Kafka connections, ~0.35 MB RSS and ~2.5–3 m-cores of fixed cost at 10 ev/s.
For comparison, a single route doing 10k ev/s costs ~0.9 cores. One shared producer client stayed at 1. PG connections stayed at 15–18
the whole time, so the per-route runtime does not touch the database. At 1000 routes the p50 latency rises from ~15 ms to 88 ms.
After the 1000 routes were archived the worker went back to 47 goroutines, 29 fds and a 56 MB live heap, so nothing leaked.

## What broke along the way
1. **Attempt 1 (tooling).** The API's per-user rate limit (1200/min, burst 300) answered 429 once more than ~150 routes were
   provisioned at deployConcurrency 16. The platform client now retries 429 after `Retry-After`. Bulk provisioning is
   therefore API-rate-bound at about 0.8–1.3 routes/s: +500 routes took 649 s. Cleanup also failed on 429 and left
   154 routes, which `cmd/stresssweep` removed.
2. **Attempt 2 (product bug, fixed).** r10–r500 passed. While r1000 was deploying, at **~595 routes** the scheduler
   expired the worker's leases all at once (`expired leases removed` count=595, 672, 714 … 1002, every 30–90 s). The worker
   dropped every runner and redeployed it, so the routes never reached a steady state. The cause was in
   `reconcile.Reconciler.Run`: heartbeats (the only thing that extends leases, TTL 30 s) ran in the same select loop as
   `Reconcile`, and Reconcile starts runners one after another. A pass over hundreds of new routes took longer than the
   lease TTL, no heartbeat went out, and the leases expired. Heartbeats now run on their own goroutine
   (`reconcile.Reconciler.heartbeats`), with the test `TestHeartbeatsContinueDuringLongReconcile`. Attempt 3 had zero lease expiries at 1000 routes.
3. Scheduler pass duration and hot-reload latency are not exposed by any metric or log. "deploy→running" (API deploy → observedState
   running) is the end-to-end proxy. Its tail at 1000 routes (p95 32 s, max 75 s) comes from the API rate limit and from the
   serial `Reconcile` pass, which starts runners one at a time.
