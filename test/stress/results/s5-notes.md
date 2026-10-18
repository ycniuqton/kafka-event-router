# S5 rule complexity: notes

Setup: local Kafka on cores 6-9, worker pinned to cores 0-5 (GOMAXPROCS 6), loadgen on cores 10-13. One route, 12
partitions, 1 KB JSON, at_least_once, acks=all. The default `maxInFlight` is 1000, which is the S2 p12 configuration
(knee 65k). Filters are N always-true conditions joined by AND. Steps are N `set` steps. JS is an identity
`transform` step.

The runs used three passes:
- `s5-explore`: 1-min steps that locate each case's own knee.
- `s5-confirm` and `s5-confirm-s5`: 5-min steps with the full criteria (lag slope after 30 s **and** p99 < 1 s).
- `s5-explore-cpu`: the same shapes with `maxInFlight` 5000, so the route is bound by CPU instead of the in-flight
  window.

Raw data: `s5-explore/`, `s5-confirm/`, `s5-confirm-s5/`, `s5-explore-cpu/`.

## Default maxInFlight 1000

| config | max sustained, 5 min (ev/s) | p99 at max | first failing | worker CPU at 40k (cores) | CPU µs per event at 40k |
|---|---|---|---|---|---|
| baseline (no rules) | 65 000 (S2 p12) | 291 ms (S2) | 72.5k lag (explore) | 2.52 | 63 |
| filter 1 | 65 000 (explore) | 860 ms | 72.5k lag | 2.64 | 66 |
| filter 10 | 65 000 (explore) | 770 ms | 72.5k p99 | 2.63 | 66 |
| filter 50 | **65 000** | 631 ms | 72.5k lag (explore) | 2.63 | 66 |
| steps 1 | 65 000 (explore) | 274 ms | 72.5k lag | 2.55 | 64 |
| steps 5 | **57 500** | 860 ms | 65k: p99 1.29 s (5 min) | 2.85 | 71 |
| steps 20 | **50 000** | 647 ms | 57.5k: lag +1.2k/s | 3.92 | 98 |
| JS identity | **5 000** | 188 ms | 6.5k: lag +970/s | 4.21 at 5k | 842 |
| f10 + s5 + JS | **4 000** | 26 ms | 5k: p99 19 s | 3.75 at 4k | 937 |
| f50 + s20 + JS | **4 000** | 434 ms | 5k: lag +630/s | 4.37 at 4k | 1 092 |

Every run lost 0 events and produced 0 duplicates.

## CPU bound (maxInFlight 5000, 1-min explore only)

| config | max sustained (ev/s) | worker CPU at 50k | vs baseline |
|---|---|---|---|
| baseline | 87 500 | 2.98 | — |
| filter 50 | 72 500 | 3.39 | CPU +14 %, max −17 % |
| steps 5 | 72 500 | 3.63 | CPU +22 %, max −17 % |
| steps 20 | 57 500 | 4.70 | CPU +58 %, max −34 % |
| f50 + s20 | 50 000 | 4.93 | CPU +65 %, max −43 % |

## Findings

- **Filters are cheap.** With the default in-flight window, 50 conditions do not move the knee (65k). The CPU per
  event rises by ~4 % at 40k and by ~14 % at 50k.
- **Recipe steps cost ~2 µs·core per `set` step per event.** That is about 63 → 71 → 98 µs from 0 to 5 to 20 steps.
  - With the default window, 5 steps lower the knee to 57.5k and 20 steps lower it to 50k.
  - In the CPU-bound regime they cost 17 % and 34 % of the throughput.
- **JS is the cliff.** An identity JS step costs ~0.84 ms·core per 1 KB event, 13× the whole non-JS pipeline. One
  route tops out at **5k ev/s**.
- **The JS limit is the instance pool, not the worker CPU.** At the knee the worker uses only ~4.2–4.5 of its 6 cores,
  and goroutines climb to ~300.
  - The cause is the per-route sandbox pool: `DefaultSandboxParallelism` = 4 in `internal/runtime/reconcile/compiler.go`
    and `Max: scope.Parallelism` in `pkg/sandbox/manager.go`.
  - At most 4 JS executions run at once per route: 4 × ~1.25k ev/s ≈ 5k ev/s.
  - Lanes queue behind the pool slots. Steps and filters added to a JS route cost a further ~20 %.
  - This is a configuration limit, not a bug, and it was not changed.
- **Against the NFR:** even the cheapest configuration runs at ~16k ev/s per core, against a target of 100k. A JS route
  runs at ~1.2k ev/s per core.
