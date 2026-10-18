# S6 scale-out: notes

Setup: one route, 48 partitions, 1 KB, `maxInFlight` 5000, so each replica is bound by CPU rather than by the
in-flight window. Route `replicas` = number of workers.
- The extra workers `stress-worker-2/3` (metrics :8091/:8092, ids `<id>-stress-N`) are started by `scripts/workers.sh`.
- Pins come from `STRESS_WORKER_CPUS_<i>` set in the case hooks.
- Kafka runs on cores 6-9 and loadgen (3 producers + measure) on cores 10-13.
- Go 1.27 re-reads the CPU affinity, so `GOMAXPROCS` follows `taskset -acp` within about 1 s. This was checked: a
  worker re-pinned from 0-5 to 0-2 reported `go_sched_gomaxprocs_threads 3`.

Passes:
- `s6-explore`: 1-min steps.
- `s6-confirm`: 5-min steps at the explore max and knee.
- `s6-confirm-high`: further 5-min steps for fair-w2 and fair-w3, which did not fail in `s6-confirm`.

Partition ownership is snapshotted in `spread-<case>.txt` with `kafka-consumer-groups --describe --members`.

| case | workers × cores (pins) | max sustained, 5 min | first failing (5 min) | worker CPU at max | partitions per worker |
|---|---|---|---|---|---|
| fair-w1 | 1 × 6 (0-5) | **90 000** (p99 860 ms) | 100k: lag +3.0k/s | 5.10 | 48 |
| fair-w2 | 2 × 3 (0-2, 3-5) | **100 000** (p99 516 ms) | 115k: lag +4.7k/s | 2.57 / 2.75 | 24 / 24 |
| fair-w3 = c2-w3 | 3 × 2 (0-1, 2-3, 4-5) | **80 000** (p99 721 ms) | 90k: lag +9.6k/s | 1.41 / 1.33 / 1.43 | 16 / 16 / 16 |
| c2-w1 | 1 × 2 (0-1) | **40 000** (p99 434 ms) | 50k: lag +2.0k/s | 1.93 (saturated) | 48 |
| c2-w2 | 2 × 2 (0-1, 2-3) | **70 000** (p99 389 ms) | 80k: lag +9.1k/s | 1.80 / 1.87 | 24 / 24 |

Every run lost 0 events and produced 0 duplicates.

## Findings

- **Partition spread is exact.** The consumer group gives 48/24/16 partitions per replica. The scheduler places
  one replica per worker (least_loaded; all three workers report the same host). Worker CPU is balanced within ~10 %.
- **Scaling with 2 cores per worker:** 40k → 70k → 80k (1.75× and 2.0×). Per core that is 20k → 17.5k → 13.3k ev/s.
- **The same 6 cores split differently:**
  - 2 × 3 cores (100k) beats 1 × 6 cores (90k). This is likely because one process with 48 lanes pays more GC and scheduling cost; there is no pprof to confirm it.
  - 3 × 2 cores falls to 80k.
- **The upper knees were limited by the host, not the workers.**
  - At fair-w3 90k and c2-w2 80k the workers used only ~65–75 % of their cores while lag grew. Kafka was at ~1 of its
    4 cores.
  - During fair-w3 at 90k, `vmstat` (`logs/vmstat-s6-s8.log`, 15:15–15:19) shows iowait at 6–23 %, up to 10 tasks blocked on
    I/O, ~100–180 MB/s of writeback, and the page cache full (free memory ~0.5 GB of 31 GB).
  - The single local broker writes input and output, about 2 × 80–90 MB/s, to a shared VM disk. This is the same
    disk limit as S2 p12-10kb.
  - The c2-w2 80k run came before the vmstat logger was started. It has the same signature: CPU idle while lag grows
    and the producers stall.
  - So the scale-out ceiling on this box is ~90–100k ev/s, set by the broker disk and page-cache writeback.
- **Overload shape:**
  - Past the knee, throughput collapses well below capacity: c2-w2 at 80k fell to 20–40k after ~4 min.
  - Producers stall at the same time, which matches the host I/O stall above, not a worker problem.
  - Worker RSS reaches ~1 GB per worker during the collapse, from queued lanes and the producer buffer.
- **Against the NFR:** scale-out does not change the per-core efficiency, which is ~13–20k ev/s per core, against a
  target of 100k.
