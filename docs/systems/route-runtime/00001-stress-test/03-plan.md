# Route Runtime — Plan (00001-stress-test)

Incremental version on top of `00000-init`. **Unchanged:** every item RR-01..RR-209 and RR-AF-01..03 in
`00000-init/03-plan.md` keeps its state there. This plan adds only the stress-test items (`RR-ST-xx`) and the
follow-up optimisations (`RR-OPT-xx`). Mark the step you start `[~]`, finished `[x]`; never delete items.

## Tooling
- [x] RR-ST-01 `cmd/loadgen` produce (paced, step plan / ramp, 1/10/100 KB, seq + sent-ns headers) and measure (latency histogram, lost / duplicates / gaps per stream, `--drain-max`)
- [x] RR-ST-02 `cmd/stressrun` + `test/stress/kit/*`: scenario YAML, targets, create/deploy/clean `stress-` resources, sampler, knee detector, JSON + MD results, `--dry-run --cases`
- [x] RR-ST-03 Platform client retries HTTP 429 after `Retry-After`; deploy retries `topic_missing`
- [x] RR-ST-04 `cmd/stresssweep` removes leftovers of interrupted runs
- [x] RR-ST-05 Scripts: `workers.sh`, `group-spread.sh`, `host-probe.sh`, `staging/watch.sh`, `staging/smoke.sh`; `infra.sh` local stress Kafka; `s1-bench.sh` + `s1-report.py`
- [x] RR-ST-06 `kafka.topicConfig` for stress topics (256 MiB/partition, 32 MiB segments, 30 min retention)
- [x] RR-ST-07 Two-pass scenarios (explore 1-min / confirm 5-min) for S2, S5, S6; S7 rewritten at 45k; staging `stg-s2/s3/s8`
- [x] RR-ST-08 `test/stress/README.md` (local pinning, staging-on-host section)

## Scenarios
- [x] RR-ST-10 S1 engine only — `results/s1-engine.json/.md`, profiles in `s1-profiles/`
- [x] RR-ST-11 S2 single route — `s2-explore/`, `s2-confirm.json`, `s2-confirm-final/`, `s2-notes.md` (p3/p48/mif100/mif5000/10 KB confirmed on the earlier 4096-record lane caps)
- [x] RR-ST-12 S3 many routes to 1000 — `s3-many-routes.json/.md`, `s3-notes.md`
- [x] RR-ST-13 S4 fan-out N = 1..50 — `s4-fanout.json/.md`, memory A/B files
- [x] RR-ST-14 S5 rule complexity — `s5-explore/`, `s5-explore-cpu/`, `s5-confirm/`, `s5-confirm-s5/`, `s5-notes.md`
- [x] RR-ST-15 S6 scale-out 1→3 workers — `s6-explore/`, `s6-confirm/`, `s6-confirm-high/`, `s6-notes.md`
- [x] RR-ST-16 S7 failure under load (0 lost in all 7 cases) — `s7/`, `s7-probe/`, `s7-eos/`, `s7-notes.md`
- [x] RR-ST-17 S8 local mini-soak 20 min — `s8-mini-soak/`, `s8-mini-soak-notes.md` (no leak; p99 failed on host disk stalls)
- [x] RR-ST-18 Staging S2 / S3 / S8 (40 min soak passed) — `results/staging/`
- [ ] RR-ST-19 S8 full 8-hour soak — blocked: needs a dedicated or local disk for the broker (D-ST-3)
- [ ] RR-ST-20 Staging S3 beyond 89 routes — needs a worker redeploy with `ROUTER_WORKER_CAPACITY` raised
- [ ] RR-ST-21 Staging re-run on the build with RR-ST-30/31 (staging worker is the 2026-10-16 build)
- [ ] RR-ST-22 Re-confirm S2 p3 / p48 / mif100 / mif5000 / 10 KB on the final lane caps
- [ ] RR-ST-23 Re-run S1 on a quiet host (part of the run overlapped a parallel `go build`)

## Bugs fixed during the test
- [x] RR-ST-30 Lane pause mark below poll size drained backlog at ~185 ev/s/partition — `FetchMaxWait` 500 ms, lane marks 2048/1024 records + 2/1 MiB (`runner/types.go`, `partition_lane.go`, `source_consumer.go`); tests `runner_backlog_test.go`, `partition_lane_test.go`
- [x] RR-ST-31 Lease expiry storm at ~600 routes — heartbeats on their own goroutine (`reconcile/reconciler.go`); test `TestHeartbeatsContinueDuringLongReconcile`

## Follow-up optimisations (priority order, see design §7)
- [ ] RR-OPT-01 Faster JSON codec: pooled buffers, zero-reflection decode/encode, lazy field decode, raw pass-through when no transform — expected 2–3× per core
- [ ] RR-OPT-02 Adaptive or higher default `maxInFlight` (5000 or sized from ack latency) — expected +25–35 % single route
- [ ] RR-OPT-03 GOMEMLIMIT from the container limit + worker-wide lane byte budget — bounded RSS under overload
- [ ] RR-OPT-04 Shared source per (connection, topic): decode once, evaluate N routes — fan-out 3–5× at N = 10–50
- [ ] RR-OPT-05 Configurable JS sandbox pool (default = cores) and payload-scaled JS timeout — JS routes ~5k → ~7.5k on 6 cores
- [ ] RR-OPT-06 Calibrate scheduler load units from measured per-route cost; parallel runner starts in reconcile; bulk-deploy rate limit — > 500 routes per default worker, deploy p95 to a few s
- [ ] RR-OPT-07 Staging sizing (worker ≥ 2 CPU, Kafka ≥ 2 CPU) and root cause of the ~8-min Kafka latency spike
- [ ] RR-OPT-08 Exactly-once: larger transactions per poll, overlap commit with next batch — EOS above 31k
- [ ] RR-OPT-09 Worker `/debug/pprof`, scheduler cycle and reload latency metrics
- [ ] RR-OPT-10 Investigate the steady consumer-lag level doubling after a worker restart (S7, harmless)
- [ ] RR-OPT-11 Fix phantom `insufficient_samples` step in the knee table after a stop (cosmetic, tooling)
