# S4 fan-out (local)

N routes share one 12-partition input topic at 5000 ev/s (1 KB). Each route has its own group and output topic, and delivery is
checked per output topic (expected = N × acked). Raw data is in `s4-fanout.json`: n1/n5/n10 are 5-min steps; n25/n50 are 1-min segments with `stopOnKnee`.

| N | output ev/s offered | output ev/s achieved | sustained | p50 / p99 ms | worker CPU (of 6) | Kafka CPU | worker RSS max MB | goroutines | lost / dup |
|---|---|---|---|---|---|---|---|---|---|
| 1 | 5 000 | 5 000 | yes | 13 / 20 | 0.48 | 0.32 | 107 | 74 | 0 / 0 |
| 5 | 25 000 | 25 000 | yes | 17 / 28 | 1.88 | 0.87 | 113 | 170 | 0 / 0 |
| 10 | 50 000 | 50 000 | yes | 23 / 97 | 3.60 | 1.61 | 165 | 289 | 0 / 0 |
| 25 | 125 000 | 117 000 | **no**: lag +8.2k/s, p99 15 s | 1851 / 15073 | **5.84 (saturated)** | 1.22 | 4 810 | 667 | 0 / 0 |
| 50 | 250 000 | 121 000 | **no**: lag +124k/s, p99 38 s | 14942 / 37749 | **5.88 (saturated)** | 0.86 | 16 077 | 1323 | 0 / 0 |

- Fan-out cost is linear in N. Each route decodes and encodes the same input on its own, so worker CPU per *output*
  event (~72 µs) is the same as for a single route. The fan-out ceiling of one 6-core worker is **~117–121k output ev/s**,
  bounded by worker CPU. With N=25 at 5k ev/s that is 4.7k input ev/s; with N=50 it is 2.4k.
- **Memory under overload:** RSS reached 4.8 GB (N=25) and 16 GB (N=50) while the lanes, the fetch buffers and the GC were all
  saturated. It is not a leak: once the routes were archived, the live heap after GC was 56 MB, but RSS stays high until the
  scavenger returns the memory. Same-load A/B at N=25 (`s4-memory-ab-*.json`): lane caps of 512 records gave 1.5 GB, the final
  caps of 2048 records / 2 MiB gave 3.4 GB, and 4096 / 4 MiB (used for the table above) gave 4.8 GB. Worst-case worker memory grows
  with routes × partitions × lane cap, and there is no worker-level memory budget (no GOMEMLIMIT).
- The first n25/n50 attempt failed with HTTP 422 `topic_missing`. The API validated the deploy before the freshly created
  output topics were visible to it. `Platform.Deploy` now retries that rejection for up to 1 min.
