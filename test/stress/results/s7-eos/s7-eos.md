# s7-eos (local)

S7 follow-up — exactly_once + kill -9 worker at 22k ev/s, ~70% of the ~31k ev/s an exactly_once route sustained in s7 exactly-once-kill-worker (where 45k was above its capacity). Output read with read_committed; zero loss and zero duplicates required. Local only.


Run `stress-s7-101718181580` on vps-c0215fee (14 CPUs), 2026-10-17T16:18:15-02:00 → 2026-10-17T16:24:38-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| eos-kill-worker-22k | 1 | 0 | 22000 | p99_over_limit | 50855.9 | 0 | 0 | 2067 | 1.98 |  |

## eos-kill-worker-22k

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 22000 | 21997 | -867.8 | 12609 | 50855.9 | p99_over_limit |
