# Route Runtime — Requirements (00001-stress-test)

Incremental version on top of `00000-init`. **Unchanged:** every functional requirement FR-01..FR-36, the phase
tags and the NFRs of `00000-init/01-requirements.md` still apply as written. This version adds only the stress-test
goals, scenarios and pass criteria used to measure the limits of the runtime design.

## Business intent
Before production sizing we need hard numbers: how many events per second one core, one worker and one staging
stack can carry; how many routes one worker can hold and what breaks first; what rule complexity costs; whether
scale-out is linear; and proof that failures under load lose no events.

## Goals
- ST-G1 Max sustainable throughput per core, per worker and per staging stack (1 KB JSON reference payload).
- ST-G2 Routes per worker and per system, and the first resource that runs out.
- ST-G3 Cost of rule complexity (filter conditions, recipe steps, JS) in CPU per event and in max rate.
- ST-G4 Scale-out efficiency from 1 to 3 workers.
- ST-G5 Zero lost events under worker crash, broker restart and heavy DLQ traffic.
- ST-G6 Stability over a soak at 70 % of the single-route maximum (no RSS / goroutine growth).
- ST-G7 Ranked list of bottlenecks with evidence, and fixes in priority order with expected gain.
- Reference: earlier benchmark ~23–28k ev/s/core at 1 KB (~85 % JSON codec); NFR target 100k ev/s/core.

## Scenarios
| Id | Scenario | Variables |
|---|---|---|
| S1 | Engine only (benchmark, 1 pinned core) | step type × payload 1 / 10 / 100 KB |
| S2 | Single route end-to-end ramp | partitions 1 / 3 / 12 / 48, `maxInFlight` 100 / 1000 / 5000, 10 KB |
| S3 | Many routes, 10 ev/s each | 10 → 50 → 100 → 250 → 500 → 1000 routes |
| S4 | Fan-out: N routes on one topic | N = 1 / 5 / 10 / 25 / 50 at 5k ev/s input |
| S5 | Rule complexity | filter 1 / 10 / 50 conditions, steps 1 / 5 / 20, ± JS |
| S6 | Scale-out | 1 → 2 → 3 workers, equal total cores and 2 cores each |
| S7 | Failure under load (70 % of knee) | kill -9 worker, Kafka graceful / kill -9 restart, 50 % DLQ, exactly-once |
| S8 | Soak at 70 % of S2 max | 20 min local mini-soak, 40 min staging soak (8 h deferred) |

## Pass criteria
- PC-1 A rate is *sustained* only if, for the whole step after a 30 s warm-up, the consumer-lag slope is
  ≤ max(50, 1 % of rate) ev/s **and** p99 end-to-end latency (producer stamp → output consumer) is < 1 s.
- PC-2 The knee is found with 1-min steps, then confirmed with 5-min steps around it (S2, S5, S6).
- PC-3 Achieved producer rate must be ≥ 95 % of target, otherwise the step is a load-generator limit, not a result.
- PC-4 Every run counts each event by stream + sequence header: **lost = 0 is mandatory**; duplicates are allowed
  for at-least-once and must be 0 for exactly-once.
- PC-5 Soak: RSS and goroutines flat (no trend), lag returns to baseline after spikes, overall p99 < 1 s.
- PC-6 Every stress resource (topic, route, connection, consumer group) is prefixed `stress-` and removed or
  archived afterwards; the shared staging Kafka is never touched outside that prefix.
- PC-7 Staging runs never regenerate secrets and redeploy only if strictly needed; the orders-smoke route must
  pass before, during and after the staging run.

## Environment constraints
- Local: 14 cores / 31 GB, worker pinned to 6 cores, Kafka to 4, load generator to 4; one shared VM disk.
- Staging: worker container 1.0 CPU, Kafka container 1.0 CPU, shared with dev/staging traffic.

## Out of scope
- Product changes beyond bugs found by the test (optimisations are follow-up plan items).
- Multi-broker Kafka clusters and Avro/Protobuf payloads.
