# S7 failure under load: notes

Setup: one route, 12 partitions, 1 KB, default `maxInFlight` 1000, `onError: dlq`. The measurer reads the output
**and** the DLQ, so every acked event must arrive in one of the two. The load is 45k ev/s, ~70 % of the 65k p12 knee.
The JS DLQ case runs at 3.5k (70 % of the S5 JS knee). Each run is a 6-min step, with the failure at +90 s.

Raw data:
- `s7/`: first pass of all cases.
- `s7-probe/`: kill-worker repeated with a host I/O probe and partition snapshots, plus a 30k case.
- `s7-eos/`: exactly_once at 22k, ~70 % of its own capacity.
- `logs/vmstat-s6-s8.log`: host vmstat every 5 s.

"Output back" is the time from the failure until delivery is at ≥ 50 % of the offered rate again. "Recovered" is the
time from the failure until consumer lag is back at its pre-failure level.

| case | acked | lost | duplicates | output back | recovered | catch-up rate | notes |
|---|---|---|---|---|---|---|---|
| kill -9 worker, restart after 10 s (s7-probe) | 16 199 948 | **0** | 15 421 | 50 s | **130 s** | ~71k ev/s | output resumes after the dead member's group session timeout (~45 s); peak lag 2.0M |
| same, at 30k | 10 799 995 | **0** | 9 966 | 50 s | ~90 s | ~71k ev/s | |
| kill -9 worker (first pass, s7) | 16 199 975 | **0** | 6 531 | 50 s | 435 s (after load end) | erratic 10–70k | host I/O stall during catch-up (iowait 10–29 %, up to 11 tasks blocked); not reproduced in s7-probe |
| graceful Kafka restart | 16 199 979 | **0** | 15 318 | 15 s | ~40 s | ~72k ev/s | broker down ~10 s; peak lag 0.65M |
| kill -9 Kafka, start after 10 s | 16 199 969 | **0** | 12 585 | 30 s | ~85 s | ~72k ev/s | broker down ~25 s (log recovery); peak lag 1.2M |
| 50 % DLQ, recipe error (`if_else` + `convert_type` onFailure=error) | 16 199 963 | **0** | 0 | — | — | — | sustained at 45k, p99 166 ms; out 8 100 813 + DLQ 8 099 150 |
| 50 % DLQ, JS `throw` | 1 260 000 | **0** | 0 | — | — | — | sustained at 3.5k, p99 705 ms; out 629 869 + DLQ 630 131 |
| exactly_once + kill -9 at 45k | 16 199 986 | **0** | **0** | 50 s | 420 s (after load end) | ~31k ev/s | 45k is above what exactly_once sustains (~31k at 1.4 cores) |
| exactly_once + kill -9 at 22k (s7-eos) | 7 920 002 | **0** | **0** | 50 s | ~190 s | ~31k ev/s | read_committed; peak lag 1.0M |

## Findings

- **Zero loss in every case.** Every acked event reached the output or the DLQ, with no gaps per stream, including
  kill -9 of the broker under acks=all on a single node. No product bug was found.
- **Duplicates (at_least_once):** 6–15k per failure. That is about the in-flight window plus the uncommitted tail
  (~0.15–0.35 s of traffic).
- **exactly_once:** 0 duplicates and 0 loss under kill -9 with read_committed.
  - The cost is throughput: about 31k ev/s at ~1.4 cores, because of one transaction per poll.
  - Catch-up is just as slow, so recovery takes ~3 min at 22k.
- **Recovery after a worker crash is dominated by the consumer-group session timeout.**
  - The dead process's member keeps its 12 partitions until it is evicted. Output resumes ~45 s after the kill, even
    though the new worker is ready in ~1 s.
  - Catch-up then runs at the `maxInFlight` cap (~71k), so recovery ≈ 45 s + backlog ÷ (71k − offered rate).
- **Recovery after a broker restart is quick:** 40–85 s.
- **Group lag level after a restart:** the steady group lag rose from ~8–16k to ~26–29k at the same rate, from
  committed offsets. It is harmless but visible on dashboards. It is probably the commit cadence; this was not
  investigated further.
- **Host I/O sensitivity:**
  - The slow, erratic catch-up in the first kill-worker run, the S6 upper knees and the two stalls in the mini-soak
    all coincide with iowait of 10–29 % and blocked tasks on the shared VM disk.
  - Writeback was only ~50–180 MB/s at those times, and steal reached 15 %. The local Kafka writes every event twice,
    as input and output.
  - These are environment limits, not router behaviour. The worker recovers at full speed once the disk does.
