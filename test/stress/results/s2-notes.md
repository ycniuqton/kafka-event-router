# S2 single route end-to-end: notes

Setup: local Kafka 4.4 on cores 6-9, worker pinned to cores 0-5 (GOMAXPROCS 6), loadgen on 10-13, 1 KB JSON
(10 KB where noted), no filter or transform, at_least_once, acks=all. Two passes:
`s2-explore` (1-min steps, 20 s warm-up) finds the knee, then `s2-confirm` checks it with 5-min steps and the full criteria (lag slope
≤ max(50, 1 % of rate)/s after 30 s **and** p99 < 1 s). Raw data: `s2-explore/<case>/`, `s2-confirm.json`, and
`s2-confirm-final/` (p1 and p12 re-run with the final lane caps of 2048 records / 2 MiB). `s2-explore-before-fix/` holds the first pass, before the fix.

| case | max sustained 5 min (ev/s) | p99 at max | first failing (from explore) | worker CPU at max | binding resource |
|---|---|---|---|---|---|
| p1 (1 partition) | **30 000** (final build) | 471 ms | 35k: lag +4.1k/s | 1.74 | one partition lane = one goroutine: decode → process → encode on ~1 core |
| p3 | ≥ 57 500 | 508 ms | 65k: lag +2.7k/s | 3.33 | 3 lanes plus the in-flight limit (see p12) |
| p12, maxInFlight 1000 | **65 000** (final build) | 291 ms | 80k (pre-fix explore), and 65k p99 1.36 s with 8 MiB caps | 3.77 | `limits.maxInFlight`: 1000 in flight ÷ ~15 ms ack time ≈ 66k/s (Little's law) |
| p48 | ≥ 57 500 | 303 ms | 65k: lag +1.3k/s | 3.76 | in-flight limit; Kafka CPU 1.4 cores (48-partition fetch/produce) |
| p12, maxInFlight 100 | 8 000 | 59 ms | 10k: lag +1.5k/s | 0.71 | in-flight limit: 100 ÷ ~12 ms ≈ 8.5k/s, with the CPU almost idle |
| p12, maxInFlight 5000 | **80 000** | 246 ms | 90k: p99 3.1 s | 4.60 (of 6) | worker CPU (codec and GC), plus the shared broker |
| p12, 10 KB | ≥ 20 000 (200 MB/s in + 200 MB/s out) | 754 ms | 25k: producers reached only 16.7k (explore) | 3.49 | broker disk: in + out ≈ 400 MB/s, against 405 MB/s measured by dd |

Zero loss and zero duplicates in every case, including all overloaded steps after drain.

Worker efficiency: ~17–18.5k ev/s per busy core at 1 KB (p1: 30k on 1.74 cores; p12-mif5000: 80k on 4.6 cores) and
~5.7k ev/s per core at 10 KB. The router does not reach the NFR of 100k ev/s per core: it is 5–6× short. One 6-core worker tops out at ~80–87k ev/s at 1 KB,
and only when `maxInFlight` is raised; the default of 1000 caps a single route at ~65k ev/s on this broker.

Profiling: the worker has no `/debug/pprof`, so the profiles are the S1 benchmark ones (`s1-profiles/`, recipe-5 1 KB). JSON
decode is 47.6 % and JSON encode 39.6 % of CPU, and allocation/GC frames lead the flat top 15. That matches the CPU-bound knee of
p12-mif5000. Kafka never exceeded 1.4 of its 4 cores.

## Product bug found and fixed: backlog drains at ~185 ev/s per partition
Symptom (first explore pass): once a step overloaded the route, the backlog drained at **~185 ev/s per partition**
(~360 ev/s for the route) with the worker CPU near 0. The drain timed out, the measurer never reported, and one
partition starved while the others kept up.
Cause (`internal/runtime/runner`):
1. `MaxPollRecords` (1000) was larger than `LaneHighWater` (512), so one poll of a backlogged partition paused it right away.
   franz-go then drops the client's buffered records of a paused partition and has to fetch them again after the resume.
2. The re-fetch of a resumed partition waits for the fetch already in flight. That fetch covers the idle partitions and long-polls
   for franz-go's default `FetchMaxWait` of **5 s**. The result is about one poll (~1000 records) per partition every 5 s.
Fix: source consumers use `FetchMaxWait` 500 ms (`SourceFetchMaxWait`, the same as the Java default). Lane marks are now 2048 / 1024
records, above one poll, with a byte cap of 2 MiB / 1 MiB per lane (`LaneHighWaterBytes`/`LaneLowWaterBytes`).
Tests: `TestRunnerDrainsBacklogBehindIdlePartition` (kfake, 2 loaded partitions and 1 idle one, 30k records). Before the fix it drained 6k
in 12 s; after, all 30k in 2.2 s. Also `TestLane*` in `partition_lane_test.go`.
Effect: drain after overload takes ~25 s instead of never finishing, and p1 rose from 20k to 30k ev/s.
Trade-off: worst-case queued memory per partition rises from ~0.5 MB to ≤ 2 MiB (see S4 memory).
