# S8 local mini-soak (20 min at 45k ev/s): notes

Setup: one route, 12 partitions, 1 KB, default `maxInFlight` 1000, 45k ev/s (70 % of the 65k p12 knee) for 20 min,
10 s samples. Raw data: `s8-mini-soak/`. Host vmstat: `logs/vmstat-s6-s8.log`.

| window (s after start) | worker RSS mean (MB) | goroutines max | p99 mean (ms) | group lag mean | worker CPU |
|---|---|---|---|---|---|
| 120–360 (stall 1) | 212 | 78 | 69 584 | 1.7M | 2.90 |
| 360–600 | 91 | 74 | 9 858 (catch-up) | 39k | 3.00 |
| 600–840 | 87 | 74 | 68 | 12k | 2.92 |
| 840–1080 (stall 2) | 154 | 76 | 11 587 | 310k | 2.91 |
| 1080–1200 | 86 | 76 | 3 828 (catch-up) | 14k | 2.73 |
| after the run | 78 (heap 16 MB live) | 45 | — | 0 | 0 |

## Results

- **Delivery:** 53 999 988 acked, 53 999 988 unique, **0 lost, 0 duplicates**, 0 gaps.
- **No leak:**
  - RSS returns to 85–90 MB in every quiet window and goroutines stay flat at 71–78.
  - After the run the worker is back at 45 goroutines, a 16 MB live heap and 78 MB RSS, the same as before the soak.
  - Threads stayed at 13.
- **Verdict `p99_over_limit` is environmental.**
  - There were two stalls, at 16:26–16:29 and 16:40:50–16:41:40. During both, throughput fell to 11–20k with the
    worker and Kafka CPUs mostly idle.
  - Over the same seconds vmstat shows iowait of 11–24 %, up to 12 tasks blocked on I/O, and steal up to 15 %.
  - After each stall the worker caught up at ~70k ev/s and went back to p99 ≈ 25–70 ms.
  - Outside the stalls the soak met the criteria: in 600–840 s, lag was flat at ~12k and p99 was 68 ms.
- **Next step:** the 8-hour soak (`s8-soak.yaml`) should run on a host with a dedicated disk, or with the broker's
  log dir on tmpfs or local NVMe. Otherwise it will measure the VM disk, not the router.
