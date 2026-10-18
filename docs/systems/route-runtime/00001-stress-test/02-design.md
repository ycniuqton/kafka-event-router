# Route Runtime — Design (00001-stress-test)

Incremental version on top of `00000-init`. **Unchanged:** sections 1–11 of `00000-init/02-design.md` (packages,
data model, APIs, flows 5.1–5.14, configuration, metrics, decisions) still describe the runtime. This version adds
the stress-test tooling, the method, the results, the limits of the design, and two runtime fixes (§6) that change
flow 5.1 (lane backpressure) and flow 5.4 (worker heartbeat).

## 1. Tooling

| Piece | Where | Role |
|---|---|---|
| `loadgen produce` | `cmd/loadgen` | Paced open-loop producer; 1/10/100 KB JSON; stamps `stress-sent-ns`, `stress-seq`, `stress-stream` headers |
| `loadgen measure` | `cmd/loadgen` | Group-less consumer of output + DLQ topics; e2e latency histogram; lost / duplicate / gap per stream; `--drain-max` |
| `stressrun` | `cmd/stressrun` | Scenario runner: create `stress-` resources via the API, drive loadgen, sample, detect knee, clean up, write JSON + MD |
| `stresssweep` | `cmd/stresssweep` | Removes `stress-` leftovers of interrupted runs |
| Library | `test/stress/kit/` | latency, seqtrack, pacing, payload, knee, sampling, platform client (429 retry), kafkaops (topic config), runner |
| Scenarios | `test/stress/scenarios/*.yaml` | S2–S8 explore / confirm passes, `stg-s2/s3/s8` for staging |
| Targets | `test/stress/targets.yaml`, `targets-staging-host.yaml` | local and staging-on-host addresses, process sources, `kafka.topicConfig` |
| Scripts | `test/stress/scripts/` | `workers.sh`, `group-spread.sh`, `host-probe.sh`, `staging/watch.sh`, `staging/smoke.sh` |
| S1 bench | `test/stress/s1-bench.sh`, `s1-report.py` | Go benchmarks of `pkg/engine` + runner path, pinned to 1 core |

### 1.1 Scenario run flow

```
stressrun --scenario <yaml> --target <local|staging>   [user action]
└─ load scenario + target → validate prefix `stress-`
   └─ per case
      └─ create topics (stress-<run>-<case>-*) with kafka.topicConfig
      └─ create routes via API → deploy → poll until observed=running   (429 → wait Retry-After; topic_missing → retry)
      └─ start loadgen measure [async]  → counts every output + DLQ record
      └─ start loadgen produce [async]  → step plan (rate × duration)
      └─ sampler every 5 s [cron]       → lag, rates, p99, worker/Kafka CPU + RSS, goroutines, conns
      └─ actions (kill worker, restart Kafka) at scheduled offsets [async]
      └─ knee detector per step
         └─ lag slope ≤ limit AND p99 < 1 s → verdict sustained
         └─ lag slope > limit              → verdict lag_growing → stop ramp (knee)
         └─ p99 ≥ 1 s                      → verdict p99_over_limit → stop ramp (knee)
      └─ drain → measure summary (lost, duplicates) → archive routes, delete topics/groups
   └─ write results/<scenario>.json + .md
```

## 2. Environment

| | Local | Staging |
|---|---|---|
| Host | 14 cores, 31 GB, one shared VM disk (~405 MB/s) | 104.234.195.126, compose `event-router-staging` |
| Worker | 6 cores pinned (0-5), GOMAXPROCS follows mask | container limit 1.0 CPU, build of 2026-10-16 |
| Kafka | single broker, cores 6-9 | single broker, container limit 1.0 CPU, shared with dev |
| Load generator | cores 10-13 | runs on the host against 127.0.0.1:29092 |
| Postgres / Redis | staging PG via tunnel (`router_dev`), Redis db 1 | in stack |

## 3. Method
- Reference payload 1 KB JSON, 12 partitions, at-least-once, `onError=dlq`, default `maxInFlight` 1000.
- Knee search: 1-min steps until the first failing step, then 5-min confirmation steps around it (PC-1, PC-2).
- Every event carries a stream + sequence header, so loss and duplicates are counted exactly, not estimated.
- Bottleneck attribution: worker and Kafka CPU per sample, S1 CPU profiles (the worker has no `/debug/pprof`),
  vmstat / `host-probe.sh` for iowait, Little's law for in-flight caps.

## 4. Results

### 4.1 Headline capacity
| Measure | Value |
|---|---|
| Engine only, 1 core, 1 KB (S1) | 10–15.6k ev/s (filter-only 15.6k; 87 % of CPU in JSON decode + encode) |
| End-to-end per core, 1 KB (S2) | ~17–18.5k ev/s (10 KB: ~5.7k); NFR 100k → **5–6× short** |
| One route, one 6-core worker | 30k (1 partition) / 65k (12 p, default in-flight) / **80k** (in-flight 5000) |
| One worker, many routes or fan-out | ~90k input (S6) / ~120k output ev/s (S4, CPU saturated) |
| Local system, 2 workers | **100k ev/s** (host disk is the ceiling) |
| Staging stack | **10k ev/s** (worker 1 CPU limit), 11–12k ev/s per core |
| Routes per worker (local, cap 1200) | **1000 routes** at 10 ev/s pass (p99 244 ms); deploy p95 32 s |
| Routes per worker (staging) | ~89 (scheduler capacity 100 units ÷ 1.1 per route) |
| Lost events, all scenarios | **0** |

### 4.2 S1 engine only (1 pinned core, 3 s × 3 runs, spread ≤ 3 %)
| Shape | 1 KB ev/s | 10 KB | 100 KB | allocs/op (1 KB) |
|---|---|---|---|---|
| filter-only | 15.6k | 1.96k | 213 | 260 |
| filter 1 / 10 / 50 | 14.4k / 13.9k / 14.0k | ~1.85k | ~203 | 260 |
| steps 1 / 5 / 20 | 14.1k / 13.2k / 10.0k | ~1.8k | ~203 | 263 / 278 / 331 |
| recipe-5 | 13.1k | 1.93k | 208 | 276 |
| mapper | 11.3k | 1.52k | 158 | 319 |
| js / steps-5 + js | 814 / 784 | 132 | 14 | 832 |

Profile: JSON decode 47.6 %, encode 39.6 %; allocation/GC lead the top 15. JS ≈ 1.2 ms per 1 KB event, ~70 ms at
100 KB (over the 50 ms default JS timeout). Caveat: a parallel `go build` ran during part of the run (quick check
gave 21.7k for filter-only).

### 4.3 S2 single route (5-min confirmed)
| Case | Max sustained | p99 at max | Fails at | Worker CPU (of 6) | Ran out first |
|---|---|---|---|---|---|
| p1 | 30,000 | 471 ms | 35k lag | 1.74 | one partition lane ≈ 1 core of codec |
| p3 | ≥ 57,500 | 508 ms | 65k lag | 3.33 | 3 lanes + in-flight cap |
| p12, mif 1000 | 65,000 | 291 ms | 80k | 3.77 | `maxInFlight` (1000 ÷ ~15 ms ack ≈ 66k/s) |
| p48 | ≥ 57,500 | 303 ms | 65k lag | 3.76 | in-flight cap; Kafka 1.4 cores |
| p12, mif 100 | 8,000 | 59 ms | 10k | 0.71 | in-flight cap (100 ÷ 12 ms) |
| p12, mif 5000 | **80,000** | 246 ms | 90k (p99 3.1 s) | 4.60 | worker CPU (codec + GC) |
| p12, 10 KB | ≥ 20,000 | 754 ms | 25k | 3.49 | broker disk (~400 MB/s in + out) |

Commit lag was never the limit.

### 4.4 S3 many routes (10 ev/s per route, worker capacity 1200)
| Routes | p50 / p99 ms | Worker CPU | Kafka CPU | RSS MB | Goroutines | Kafka conns | PG conns | deploy→running p50 / p95 / max s |
|---|---|---|---|---|---|---|---|---|
| 10 | 18 / 19 | 0.08 | 0.11 | 67 | 177 | 53 | 18 | 1.2 / 1.3 / 1.3 |
| 50 | 13 / 19 | 0.23 | 0.28 | 83 | 699 | 232 | 18 | 1.7 / 2.4 / 2.4 |
| 100 | 13 / 20 | 0.38 | 0.45 | 102 | 1,349 | 402 | 18 | 1.9 / 3.7 / 3.8 |
| 250 | 15 / 23 | 0.79 | 0.88 | 154 | 3,302 | 1,052 | 18 | 3.3 / 8.3 / 9.8 |
| 500 | 19 / 39 | 1.53 | 1.61 | 235 | 6,551 | 1,645 | 18 | 5.8 / 13.9 / 21.8 |
| 1000 | 88 / 244 | 3.00 | 2.76 | 416 | 13,073 | 3,042 | 18 | 10.5 / 32.5 / 74.7 |

Per route: ~13 goroutines, ~3 Kafka connections, ~0.35 MB, ~3 m-cores worker + ~3 m-cores Kafka. No leak (47
goroutines, 56 MB live heap after archive). Before fix 6.2 the worker broke at ~595 routes (lease expiry storm).
Deploy throughput is bounded by the API per-user rate limit (1200/min): +500 routes took 649 s.

### 4.5 S4 fan-out (5k ev/s into one 12-partition topic)
| N | Output ev/s | Result | p99 | Worker CPU | RSS max |
|---|---|---|---|---|---|
| 1 | 5,000 | sustained | 20 ms | 0.48 | 107 MB |
| 5 | 25,000 | sustained | 28 ms | 1.88 | 113 MB |
| 10 | 50,000 | sustained | 97 ms | 3.60 | 165 MB |
| 25 | 117k of 125k | lag growing | 15 s | 5.84 (saturated) | 4.8 GB |
| 50 | 121k of 250k | lag growing | 38 s | 5.88 (saturated) | 16 GB |

Cost is linear in N because each route decodes the same input. RSS spikes under overload are queued lanes, not a
leak (56 MB live heap after GC); there is no GOMEMLIMIT or worker memory budget.

### 4.6 S5 rule complexity (12 p, 1 KB, mif 1000)
| Config | Max sustained (5 min) | p99 at max | CPU per event at 40k |
|---|---|---|---|
| baseline | 65,000 | 291 ms | 63 µs |
| filter 1 / 10 / 50 | 65,000 | 860 / 770 / 631 ms | 66 µs |
| steps 1 / 5 / 20 | 65,000 / 57,500 / 50,000 | 274 / 860 / 647 ms | 64 / 71 / 98 µs |
| JS only | 5,000 | 188 ms | 842 µs |
| filter 10 + steps 5 + JS | 4,000 | 26 ms | 937 µs |
| filter 50 + steps 20 + JS | 4,000 | 434 ms | 1,092 µs |

With `maxInFlight` 5000 (1-min steps): baseline 87.5k, filter 50 72.5k, steps 5 72.5k, steps 20 57.5k,
filter 50 + steps 20 50k. A `set` step costs ~2 µs/event; filters are near free. JS costs ~13× the whole non-JS
pipeline and is capped by `DefaultSandboxParallelism` = 4 instances per route (4 × ~1.25k ≈ 5k ev/s, worker
only 4.2–4.5 of 6 cores busy).

### 4.7 S6 scale-out (48 p, mif 5000, one replica per worker)
| Case | Workers × cores | Max sustained | Fails at | CPU per worker at max |
|---|---|---|---|---|
| fair-w1 | 1 × 6 | 90,000 | 100k | 5.10 |
| fair-w2 | 2 × 3 | **100,000** | 115k | 2.57 / 2.75 |
| fair-w3 | 3 × 2 | 80,000 | 90k | 1.41 / 1.33 / 1.43 |
| c2-w1 | 1 × 2 | 40,000 | 50k | 1.93 (saturated) |
| c2-w2 | 2 × 2 | 70,000 | 80k | 1.80 / 1.87 |

Partitions and CPU split evenly. At 2 cores per worker: 40k → 70k → 80k (20k → 13k ev/s per core). The top knees
(fair-w3 90k, c2-w2 80k) are the host disk: spare worker CPU, iowait 6–23 %, up to 10 blocked tasks.

### 4.8 S7 failure under load (45k ≈ 70 % of knee)
| Case | Lost | Duplicates | Output back after | Fully recovered |
|---|---|---|---|---|
| kill -9 worker, restart after 10 s | **0** | 15,421 | 50 s | 130 s (catch-up ~71k/s) |
| same at 30k | **0** | 9,966 | 50 s | ~90 s |
| graceful Kafka restart | **0** | 15,318 | 15 s | ~40 s |
| kill -9 Kafka, start after 10 s | **0** | 12,585 | 30 s | ~85 s |
| 50 % DLQ via recipe error, 45k | **0** | 0 | — | sustained, p99 166 ms |
| 50 % DLQ via JS throw, 3.5k | **0** | 0 | — | sustained, p99 705 ms |
| exactly-once + kill -9, 22k | **0** | **0** | 50 s | ~190 s (catch-up ~31k/s) |

Worker crash recovery waits for the consumer-group session timeout (~45 s). Exactly-once sustains ~31k ev/s.

### 4.9 Soak
| Run | Events | Lost / dup | p99 | RSS / goroutines | Verdict |
|---|---|---|---|---|---|
| local mini-soak, 20 min at 45k | 54.0M | 0 / 0 | 68 ms outside 2 disk stalls (69.6 s, 11.6 s in stalls) | 78–212 MB / 45–78, no trend | no leak; p99 failed on host disk stalls (iowait 11–24 %, steal ≤ 15 %) |
| staging soak, 40 min at 7k | 16.8M | 0 / 0 | 926 ms overall; four 15 s windows 1.3–2.7 s every ~8 min | 70–139 MB / 63–75, no trend | **pass**; spikes coincide with Kafka at its 1.0 CPU limit, cause open |

### 4.10 Staging (S2 short ramp, S3 to 80 routes)
| Rate | p99 ms | Worker CPU avg / max (limit 1.0) | Kafka CPU | Verdict |
|---|---|---|---|---|
| 2,000 | 139 | 0.26 / 0.28 | 0.36 | ok |
| 6,000 | 120 | 0.53 / 0.56 | 0.46 | ok |
| **10,000** | 184 | 0.81 / 0.91 | 0.51 | max sustained |
| 12,500 | 10,355 | 0.89 / 0.98 | 0.38 | knee |

S3 staging: 10 / 50 / 80 routes, p99 79 / 166 / 3,113 ms (r80 only on a 45 s start-up spike, then 41–135 ms);
stopped at 80 because the default capacity (100 units) fits ~89 routes. Kafka spends 6–7 m-cores per route.

## 5. Limits of the design and bottleneck ranking

```
event arrives on source partition                                   [event]
└─ franz-go fetch → partition lane (one goroutine per partition, in order)
   └─ JSON decode  (47.6 % CPU)            → B1 codec-bound, per route (B4 fan-out)
   └─ filter / recipe steps (cheap)        → ~2 µs per set step
   └─ JS step → sandbox pool (4 per route) → B3 ~1.25k ev/s per instance
   └─ JSON encode  (39.6 % CPU)            → B1
   └─ in-flight limiter (maxInFlight)      → B2 rate ≤ maxInFlight ÷ ack latency
   └─ async produce acks=all [async]       → B5 broker CPU / disk
      └─ ack → MarkDone → commit every 1 s [cron]
---
worker reconcile loop (scheduler leases)                            [cron]
└─ capacity score 100 units, 1.1 per 1-partition route              → B6 ~89 routes per default worker
└─ sequential runner starts                                         → deploy→running p95 32 s at 1000 routes
```

| Rank | Bottleneck | Evidence | Effect |
|---|---|---|---|
| B1 | JSON codec CPU + allocations | S1 profile: decode 47.6 % + encode 39.6 %; 260 allocs/op; per-core 17–18k e2e | caps every CPU-bound case; 5–6× below the 100k/core NFR |
| B2 | Default `maxInFlight` 1000 | p12: 65k at mif 1000, 80k at 5000, 8k at 100 with CPU idle; matches Little's law (in-flight ÷ ~12–15 ms ack) | single route stuck at ~65k with spare CPU |
| B3 | JS sandbox pool (4 per route) | S5: JS 842 µs/event, knee 5k with 1.5–1.8 cores idle | any JS route is ≤ ~5k ev/s |
| B4 | Per-route decode on fan-out | S4: CPU linear in N, worker saturated at ~120k output ev/s; RSS to 16 GB with no memory budget | N routes on one topic cost N× and can OOM |
| B5 | Broker CPU / disk | local 10 KB knee at ~400 MB/s disk; S6 top knees with iowait; staging Kafka 0.5 CPU at 7k, at limit in spikes | system ceiling ~90–100k local, latency spikes on staging |
| B6 | Route density limits | default capacity 100 units (~89 routes), sequential starts, API rate limit 1200/min, ~3 Kafka conns per route | 1000 routes run, but deploy p95 32 s and p99 244 ms |
| B7 | Single partition lane | p1 30k = ~1 core of codec | hot single-partition topics capped |
| B8 | Exactly-once throughput | 31k sustained vs 65k at-least-once | EOS routes need 2× capacity |

## 6. Runtime changes made during the test (flows changed vs 00000-init)

### 6.1 Lane backpressure vs poll size (changes flow 5.1)
Bug: poll size 1000 > lane pause mark 512, so every poll paused the lane; franz-go dropped its buffered records and
the re-fetch waited behind the 5 s default long-poll → backlog drained at ~185 ev/s per partition with idle CPU.

```
poll returns ≤ 1000 records for partition P                          [event]
└─ lane enqueue
   └─ queued ≥ 2048 records OR ≥ 2 MiB → pause fetch on P (high mark)
   └─ queued ≤ 1024 records AND ≤ 1 MiB → resume fetch on P (low mark)
└─ fetch on idle partitions waits ≤ 500 ms (FetchMaxWait), not 5 s
```
Files: `internal/runtime/runner/types.go`, `partition_lane.go`, `source_consumer.go`. Tests:
`runner_backlog_test.go` (30k drained in 2.2 s, was 6k in 12 s), `partition_lane_test.go`. Effect: p1 20k → 30k.
Trade-off: worst-case queued memory per partition 0.5 MB → 2 MiB.

### 6.2 Heartbeat independent from reconcile (changes flow 5.4)
Bug: heartbeats ran in the same loop as `Reconcile`, which starts runners one by one; a pass over ~600 routes ran
past the 30 s lease, the scheduler expired every lease of the worker and all routes restarted in a loop.

```
worker start                                                        [user action]
└─ heartbeat goroutine [async] every 10 s → renew leases (never blocked by reconcile)
└─ reconcile loop [cron] 15 s + NOTIFY [event] → start / stop / upgrade runners (may take minutes)
```
File: `internal/runtime/reconcile/reconciler.go`. Test: `TestHeartbeatsContinueDuringLongReconcile`. Effect: 1000
routes with zero lease expiries.

### 6.3 Stress tooling only (no product effect)
API 429 retry with `Retry-After`; `topic_missing` deploy retry; `measure --drain-max`; `stresssweep`; two-pass
scenarios; `stressrun --dry-run --cases`; `kafka.topicConfig` (stress topics 256 MiB/partition, 32 MiB segments,
30 min retention).

## 7. Recommended fixes (priority order, expected gain)

| # | Fix | Addresses | Expected gain |
|---|---|---|---|
| 1 | Faster JSON codec: pooled buffers, a zero-reflection decoder/encoder, lazy decode of only the fields filters/steps touch, raw pass-through when no transform | B1, B7 | 2–3× per core (codec is 87 %); passthrough routes approach the NFR |
| 2 | Adaptive / higher default `maxInFlight` (e.g. 5000, or sized from observed ack latency × target rate) | B2 | single route 65k → 80–87k (+25–35 %) at no CPU cost |
| 3 | Worker memory budget: set GOMEMLIMIT from the container limit and a worker-wide lane byte budget | B4 | bounded RSS under overload (16 GB → budget) |
| 4 | Shared source per (connection, topic): decode once, evaluate N routes | B4 | fan-out cost from N× decode to 1× decode + N× filter (≈ 3–5× at N = 10–50) |
| 5 | Configurable JS pool size (default = cores), JS timeout scaled with payload size | B3 | JS route 5k → ~1.25k × cores (≈ 7.5k on 6 cores); fewer 100 KB timeouts |
| 6 | Calibrate scheduler load units from measured cost (3 m-cores per idle route), parallel runner starts in reconcile, bulk-deploy rate limit | B6 | default worker > 500 routes; deploy p95 32 s → a few s |
| 7 | Staging sizing: worker ≥ 2 CPU, Kafka ≥ 2 CPU; investigate the 8-min Kafka spike | B5 | staging ~20k ev/s; soak p99 stable |
| 8 | Exactly-once: larger transactions per poll, overlap commit with next batch | B8 | EOS 31k → closer to at-least-once |
| 9 | Observability: `/debug/pprof` on the worker, scheduler cycle and reload latency metrics | all | lets the next round attribute bottlenecks on the real worker |

## 8. Decisions
- D-ST-1 The 100k ev/s/core NFR is **not met** (17–18k e2e, 10–15k engine). Reaching it needs fix 1 at least.
- D-ST-2 The JS pool limit and default `maxInFlight` are configuration limits; not changed in this version.
- D-ST-3 The 8-hour soak is deferred until the broker has a dedicated disk; the local VM disk stalls would dominate.
