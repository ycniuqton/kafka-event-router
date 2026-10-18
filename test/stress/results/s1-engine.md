# S1 engine-only (full run)

`test/stress/s1-bench.sh`, 2026-10-17 04:13–04:33, Xeon Gold 6148 @ 2.4 GHz, one pinned core (`taskset -c 0`,
GOMAXPROCS=1, GC on the same core), `-benchtime 3s -count 3`. Raw: `s1-raw/`, profiles: `s1-profiles/`, JSON: `s1-engine.json`.
Run-to-run spread ≤ 3 % except where noted. Another session's `go build` ran unpinned on the box during part of the
run, so absolute numbers may be a few % low. They are also below the quick check (50 iterations, 21.7k for filter-only 1 KB).

| shape (engine: decode → process → encode) | 1 KB ev/s | 10 KB | 100 KB | allocs/op 1 KB | B/op 1 KB |
|---|---|---|---|---|---|
| filter-only | 15.6k | 1.96k | 213 | 260 | 16.8k |
| filter-1 / 10 / 50 | 14.4k / 13.9k / 14.0k | 1.89k / 1.83k / 1.86k | ~203 | 260 | 16.8k |
| steps-1 / 5 / 20 | 14.1k / 13.2k / 10.0k | 1.89k / 1.79k / 1.76k | ~203 | 263 / 278 / 331 | 17.2k–24.1k |
| recipe-5 | 13.1k | 1.93k | 208 | 276 | 18.1k |
| mapper | 11.3k | 1.52k | 158 | 319 | 23.2k |
| js (identity transform) | 814 | 132 | 14 | 832 | 51.8k |
| steps-5 + js | 784 | 134 | 14 | 856 | 53.8k |
| sandbox call alone | 879 | 142 | 15 | 522 | 29.2k |
| runner record path, filter-only / recipe-5 / mapper / js | 15.5k / 13.9k / 11.7k / 789 | 2.01k / 1.96k / 1.55k / 129 | 225 / 223 / 155 / 14 | 263–838 | |

## Takeaways
- **The codec is the cost.** recipe-5 at 1 KB: `Decode` 47.6 % + `Encode` 39.6 % of the CPU (cumulative) = 87 %,
  and allocation/GC frames (`mallocgc` 35 %, `scanObject`, `tryDeferToSpanScan`) dominate the flat top 15.
  Filter conditions are almost free (50 conditions: −10 %). Steps cost ~1.5 % each up to 5 and ~2 % each up to 20.
- **Single-core throughput is ~13–16k ev/s at 1 KB**, so it is **6–8× below the 100k ev/s/core NFR**. Cost grows
  linearly with payload: 10 KB is 8× and 100 KB is 70× slower than 1 KB.
- **A JS step costs ~1.2 ms per 1 KB event** (≈17× the whole non-JS pipeline). 37 % of samples are `_ExternalCode` (the
  compiled sandbox) and 28 % are the module call. At 100 KB that is ~70 ms per event, which is above the 50 ms default `jsTimeoutMs`.
- Inside the worker the rate per core is higher than here: 30k ev/s on one lane used 1.6–1.7 cores (S2 p1), because GC and Kafka I/O run on
  other cores. Measured end to end, the worker does 17–19k ev/s per core at 1 KB and ~5.6k ev/s per core at 10 KB.
