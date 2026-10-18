# Stress tests

Tooling to find the limits of the router: maximum sustainable throughput (consumer lag flat for the whole step **and**
p99 end-to-end latency < 1 s), routes per worker, cost of rule complexity, scale-out and behaviour under failure.

| Piece | Where | What it does |
|---|---|---|
| `loadgen produce` | `cmd/loadgen` | Paced open-loop producer (fixed rate, step plan or ramp), N partitions, key strategy, JSON template padded to 1/10/100 KB, stamps `stress-sent-ns`, `stress-seq`, `stress-stream` headers, configurable acks/linger/batch/compression, reports achieved rate. |
| `loadgen measure` | `cmd/loadgen` | Group-less consumer of the output (and DLQ) topics, optional `read_committed`; e2e latency p50/p95/p99/p99.9/max from the header; counts, duplicates and gaps per stream from the seq; JSON summary. |
| `stressrun` | `cmd/stressrun` | Runs a scenario: creates the `stress-` connection, topics and routes through the router API (deploy → wait `running`), drives loadgen processes, samples every interval, detects the knee, archives/deletes everything it created, writes `results/<scenario>.json` + `.md`. |
| Scenarios | `scenarios/*.yaml` | S2–S8 (+ `smoke`). |
| Targets | `targets.yaml` | `local` and `staging`: API/login, Kafka addresses as seen by worker / runner / loadgen, loadgen launch prefix, metric and process sources. |
| Library | `kit/` | latency histogram, seq tracker, pacing, payload, knee detector, sampling, platform API client, runner. |
| Scripts | `scripts/workers.sh`, `scripts/group-spread.sh`, `scripts/host-probe.sh`, `infra.sh`, `s1-bench.sh` | extra local workers (S6) and worker restart (S7); partition-ownership snapshots of `stress-` groups; host dirty-page/iowait probe; local stress Kafka; S1 engine benchmark. |

## Build

```sh
export PATH=$HOME/.local/go/bin:$PATH
go build -o .local/bin/loadgen ./cmd/loadgen
go build -o .local/bin/stressrun ./cmd/stressrun
go test ./test/stress/...                        # unit tests (latency math, knee, YAML, seq tracking, kfake e2e)
```

Run everything from the repository root (paths in `targets.yaml` are relative to it).

## Local set-up and CPU pinning

The 14-core box is split so the system under test, the broker and the load generator never share cores:

| Process | Cores | How |
|---|---|---|
| worker | 0-5 | start it pinned: `taskset -c 0-5 deploy/dev/run.sh restart worker` (children inherit the mask, and Go sizes `GOMAXPROCS` from it at start). `stressrun` also re-applies `taskset -acp 0-5` (targets `cpus:` hint), which pins the threads; Go ≥ 1.25 (the repo uses 1.27) re-reads the affinity and adjusts `GOMAXPROCS` within ~1 s (checked: `go_sched_gomaxprocs_threads` follows the mask). |
| Kafka (127.0.0.1:39092) | 6-9 | `test/stress/infra.sh start` (`STRESS_KAFKA_CPUS`, default 6-9). |
| loadgen (produce + measure) | 10-13 | `targets.yaml` → `loadgen.launch: [taskset, -c, "10-13"]` prefixes every loadgen process. |
| api / preview / web / stressrun | unpinned | — |

Check: `taskset -cp $(cat .local/run/worker.pid)`, `test/stress/infra.sh status`.

Prerequisites: `deploy/dev/run.sh status` all running (api :8080 dev login, worker :8081), local stress Kafka up. The
runner signs in with dev login and creates a connection with `envTag: dev` — non-production, so deploys need no
approval. (A `prod` env tag would turn deploys into approval requests; the runner reports that as an error because the
requester cannot approve their own request.)

## Running scenarios

```sh
.local/bin/stressrun --scenario smoke                      # ~30 s: 1 route, 1k events, proves the tooling
.local/bin/stressrun --scenario s2-single-route-ramp --dry-run   # merged cases, first route draft, loadgen args
.local/bin/stressrun --scenario s2-single-route-ramp --cases p12,p48
```

Flags: `--target local|staging` (default: scenario's `target`), `--cases a,b`, `--keep` (no cleanup, for inspection),
`--results <dir>`, `--env-tag`. Ctrl-C stops the load, drains, writes partial results and cleans up.

| Scenario | File | Notes |
|---|---|---|
| S1 engine only | `s1-bench.sh` | Go benchmarks of step type × payload 1/10/100 KB on one pinned core, no Kafka (`results/s1-engine.json`). |
| S2 single route ramp | `s2-single-route-ramp.yaml` | partitions 1/3/12/48, maxInFlight 100/1000/5000, 10 KB and 100 KB variants; 5-min steps 2k → 160k ev/s, `stopOnKnee`. Expect several hours for all cases; run one case at a time with `--cases`. |
| S2 two-pass | `s2-explore.yaml`, `s2-confirm.yaml` | faster S2: `s2-explore` ramps every case with 1-min steps (20 s warm-up) to locate the knee (~10 min/case), `s2-confirm` re-runs 5-min steps just below/at/above it with the full criteria. Edit the confirm steps from the explore result. |
| S3 many routes | `s3-many-routes.yaml` | incremental 10 → 1000 routes at 10 ev/s/route; restart the worker with `ROUTER_WORKER_CAPACITY=1200` first (default capacity is 100 routes). Per-route deploy→running latency is in `setup.deployToRunning`, goroutines/RSS/CPU/producer clients in the timeline. Reload latency: re-run one case with `--keep` and redeploy from the UI/API. Scheduler cycle and PG connections are not sampled: read `.local/logs/api.log` / `worker.log` and `select count(*) from pg_stat_activity where datname='router_dev'` during the run. |
| S4 fan-out | `s4-fanout.yaml` | N = 1/5/10/25/50 routes on one input topic at 5k ev/s; delivery tracked per output topic (`trackBy: topic-stream`, expected = N × acked). |
| S5 rule complexity | `s5-rule-complexity.yaml`, `s5-explore.yaml`, `s5-confirm*.yaml`, `s5-explore-cpu.yaml` | filter 1/10/50 always-true conditions, 1/5/20 `set` steps, ± identity JS; compare `maxSustainableRate` with `baseline`. Two-pass like S2 (explore 1-min, confirm 5-min); `-cpu` repeats with `maxInFlight` 5000 so rule cost shows as CPU, not hidden behind the in-flight cap. JS cases ramp 0.5k → 10k. |
| S6 scale-out | `s6-scale-out.yaml`, `s6-explore.yaml`, `s6-confirm*.yaml` | 1 → 2 → 3 workers, route `replicas` = workers; `scripts/workers.sh scale N` starts/stops `stress-worker-2..3` (:8091, :8092), pins from `STRESS_MAIN_CPUS` / `STRESS_WORKER_CPUS_<i>` set in the case hook. `fair-*` cases split cores 0-5 across the workers, `c2-*` give each worker 2 cores. Partition spread: `spread-<case>.txt`. Local only. |
| S7 failure under load | `s7-failure-under-load.yaml`, `s7-probe.yaml`, `s7-eos.yaml` | at ~70 % of the knee: kill -9 + restart worker (`scripts/workers.sh restart-main`), graceful Kafka restart, kill -9 Kafka, 50 % DLQ via recipe error (`if_else` + `convert_type`, runs at full rate) and via JS throw (at 70 % of the JS knee), exactly-once + kill (exactly_once sustains ~31k, so `s7-eos` runs it at 22k). Measures output **and** DLQ; requires `delivery.lost == 0`. `s7-probe` adds `host-probe.sh` + spread snapshots. Local only. |
| S8 soak | `s8-soak.yaml`, `s8-mini-soak.yaml` | 8 h at 70 % of the S2 p12 max (edit the placeholder rate), 15 s samples; `s8-mini-soak` is 20 min at 45k with 10 s samples. On this VM the shared disk causes multi-minute I/O stalls (iowait 10–25 %) — record `vmstat -t 5` alongside and read stalls against it. |

Scenario YAML: top-level `name`, `description`, `target`, `criteria`, `sampleInterval`, `stopOnKnee`, `incremental`,
`targetExtras`, a `defaults:` block merged under every entry of `cases:` (built-in defaults ← defaults ← case). A case
has `partitions`, `routes` (`count`, `topology: per_route|shared_input`, `replicas`, `maxInFlight`, `delivery`,
`onError`, `startFrom`, `filter: {conditions|raw}`, `steps: {count, kind: set|mask, raw}`, `js: none|identity|throw_on_bad`,
`deployConcurrency`, `extra`), `load` (`steps: [{rate, duration}]` or `ramp: {from, to, step, duration}`, `size`,
`producers`, `lanes`, `streams`, `keys`, `acks`, `linger`, `batchBytes`, `compression`, `badRatio`, `template`),
`measure` (`disabled`, `readCommitted`, `seqSource: header|payload`, `includeDlq`, `trackBy`), `drain.timeout`,
`actions: [{at, name, run}]` (shell, fired at an offset from load start) and `hooks: {before, after}`.

## Knee detection

Every `sampleInterval` (5 s) the runner records: worker `/metrics` (`router_events_consumed/produced/dlq/filtered_total`
and `router_consumer_lag` summed over this run's routes only, `go_goroutines`, `process_resident_memory_bytes`,
`process_cpu_seconds_total`, `router_worker_producer_clients`), Kafka consumer-group lag of the run's groups (franz-go
admin), `/proc/<pid>/stat` of the configured processes (CPU cores, RSS, threads), the producers' acked rate and the
measurer's window histogram. Per load step, after `warmup` (30 s):

- lag slope (least squares over the step's samples) ≤ max(`lagSlopeAbs` 50 ev/s, `lagSlopeRel` 1 % of the rate), and
- p99 of all e2e latencies of the step < `p99Limit` (1 s), and
- the producers achieved ≥ 95 % of the target (else `loadgen_below_target`: add `producers`/`lanes`, the result is not a router limit).

`maxSustainableRate` is the last step passing before the first failing one (`kneeRate`, `kneeReason`). With
`stopOnKnee` the load stops at the first failing step. Latency is open-loop (the pacer catches up after stalls), sent
time is stamped when the record is handed to the producer, so p99 includes producer batching (`linger`).

## Results

`results/<scenario>.json`: per case — merged spec, setup (routes, deploy→running latency), knee analysis per step,
delivery (`expected` acks × copies, `unique`, `lost`, `duplicates`, `gaps`, `zeroLoss`), producer and measurer
summaries, actions, cleanup, and the full 5 s timeline. `results/<scenario>.md`: one-line-per-case table plus per-step
table. Loadgen command lines and stderr: `results/logs/<scenario>/`. The file is rewritten after each case.

## Staging

The platform reaches Kafka as `kafka:19092`; loadgen runs **on the staging host** against the DEV listener
`127.0.0.1:29092`; `stressrun` runs here (Dex login with `STAGING_LOGIN_*` read one key at a time from
`~/.secrets/event-router-staging.env`, admin/lag through the dev tunnel), worker metrics and `/proc` come through
`ssh … docker exec/inspect`.

```sh
deploy/dev/tunnel.sh start
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -o .local/bin/loadgen-linux ./cmd/loadgen
H=root@$(grep '^STAGING_HOST=' ~/.secrets/event-router-staging.env | cut -d= -f2)
ssh -o BatchMode=yes $H mkdir -p /opt/event-router/stress && scp -o BatchMode=yes .local/bin/loadgen-linux $H:/opt/event-router/stress/loadgen
.local/bin/stressrun --scenario s2-single-route-ramp --target staging --cases p12
```

The staging worker container is limited to 1 CPU / 512 MB, so staging numbers are per-core-ish and not comparable
with the local 6-core worker. S6/S7 actions use local scripts and are local-only. An API token can replace Dex:
`login: token`, `token: "${env:STRESS_API_TOKEN}"`.

### Staging, run on the host

`targets-staging-host.yaml` runs `stressrun` **and** loadgen on the staging server (no tunnel, no ssh per sample):
copy static binaries (`GOOS=linux GOARCH=amd64 CGO_ENABLED=0`), the targets file, `scenarios/stg-*.yaml` and
`scripts/staging/{watch,smoke}.sh` to `/opt/event-router/stress/`, and write a 3-key `login.env` there (`STAGING_URL`,
`STAGING_LOGIN_EMAIL`, `STAGING_LOGIN_PASSWORD`, piped with `grep`, mode 600). The connection uses `envTag: dev`, the
same as the acceptance suite, so deploys need no approval and the 4-eyes policy is untouched. `kafka.topicConfig`
bounds retention of every stress topic (256 MiB/partition, 32 MiB segments, 30 min), because the broker shares a
40 GB disk. `watch.sh` samples host memory, disk, Kafka volume, PG connections and container health every 10 s into
`results/watch.csv` and sends one SIGINT to `stressrun` (graceful cleanup) if a container is not healthy, memory
> 85 %, swap is used or disk > 90 %. `smoke.sh` checks `orders-smoke` with one OrderPaid / OrderCreated pair.
The staging worker's scheduler capacity (100 load units, a 1-partition route weighs 1.1) caps S3 at ~89 routes.

## Sweeping leftovers

An interrupted run (Ctrl-C during cleanup, a crash) can leave routes, connections, topics and groups behind:

```sh
go build -o .local/bin/stresssweep ./cmd/stresssweep
.local/bin/stresssweep --prefix stress-s3- --dry-run          # list
.local/bin/stresssweep --prefix stress- --keep stress-local   # archive routes, delete connections/topics/groups
```

The prefix must start with `stress-`; `--keep` names are never touched.

## Safety

Kafka is shared with dev/staging. Everything created is named `stress-<scenario>-<stamp>-…` (connection, routes,
topics `…-in/-out/-dlq`, consumer groups `…-g`, DLQ topic set explicitly in the route so no `router.dlq.*` topic is
created). Topic and group deletion goes through a guard that refuses any name not starting with `stress-`, and only
resources registered by the run are removed. Routes are archived (pause + DELETE; there is no hard delete).

## Known limitations

- The API rate-limits each user (`ROUTER_RATE_LIMIT_PER_MINUTE`, default 1200/min, burst 300). The platform client
  retries 429 answers after `Retry-After`, so bulk provisioning (S3: 1000 routes) runs at that rate: about 1–1.5
  routes/s including the deploy→running polling, ~11 min for +500 routes.
- `loadgen measure` stops draining `--drain-max` (default 1 min) after the stop request even if records still
  trickle in, so a case whose lag never drains still reports a summary (the missing events count as lost).
- After `stopOnKnee` the step plan of a producer may already have started the next step; that step shows up in the
  per-step table as `insufficient_samples` (and collects the drain samples). It never affects the knee.

- Calibrated locally (cores 10-13, Kafka on 6-9, 12 partitions, 1 KB, acks=all): one `loadgen produce` with 4 lanes
  reaches ~280k ev/s unthrottled; one `loadgen measure` consumes ~500k ev/s using ~1.3 cores. Produce and measure
  share cores 10-13, so above ~150k ev/s end to end watch for `loadgen_below_target` and use `producers: N` (each
  gets rate/N and its own streams); 100 KB payloads are network/broker bound much earlier.
- Group lag comes from committed offsets, so it carries a constant ≈ rate × commit interval; the knee uses its slope,
  not its level. Worker `router_consumer_lag` is recorded too.
- Kill/restart actions are shell commands; their timing is ±1 sample interval. Long-running actions (probes, loops) must
  detach (`nohup … &`), because actions run through `sh -c` and their output is collected.
- `--dry-run` honours `--cases`.
- Host I/O: the local broker writes every event twice (input + output) to the VM disk. When the page cache is full and
  the shared disk slows (iowait > 10 %), everything stalls together — worker and Kafka CPU drop while lag grows and
  producers fall below target. Check `vmstat`/`host-probe.sh` before attributing such a knee to the router.
- S3 scheduler cycle time, PG connection count and hot-reload latency are not sampled automatically (see the S3 row).
- Clocks: loadgen and measurer run on the same host, so e2e latency needs no clock sync; negative values are counted as
  `negativeClockSkew`.
