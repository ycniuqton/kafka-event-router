# s5-confirm-s5 (local)

S5 confirmation, second pass for the "s5" case (5 set steps): its 5-min step at 65k failed p99 in s5-confirm, so this re-checks 57.5k with the full criteria. Same route shape as s5-confirm.


Run `stress-s5-101715243655` on vps-c0215fee (14 CPUs), 2026-10-17T13:24:36-02:00 → 2026-10-17T13:29:52-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| s5 | 1 | 57500 | 0 |  | 860.2 | 0 | 0 | 916 | 4.30 |  |

## s5

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 57500 | 57499 | 3.4 | 13860 | 860.2 | sustained |
