# smoke (local)

Tooling smoke test — 1 route, ~1k events at 100 ev/s; proves loadgen, runner, sampling, knee and cleanup end to end.

Run `stress-smoke-101706115452` on vps-c0215fee (14 CPUs), 2026-10-17T04:11:54-02:00 → 2026-10-17T04:12:16-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| one | 1 | 100 | 0 |  | 20.0 | 0 | 0 | 659 | 0.10 |  |

## one

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 100 | 100 | -0.2 | 44 | 20.0 | sustained |
