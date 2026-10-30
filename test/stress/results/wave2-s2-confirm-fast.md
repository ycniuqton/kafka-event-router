# wave2-s2-confirm (local)

Wave-2 S2 (fast codec + adaptive in-flight + memory budget): one route, 1 KB, 1-min steps (20 s warm-up, s2-explore method). Cases p1/p12 with the default (auto) in-flight limit and with maxInFlight 5000 (auto, cap 5000). Run once per ROUTER_ENGINE_CODEC (fast, tree); results are renamed wave2-s2-<codec>.json.


Run `stress-wave2-101815331695` on vps-c0215fee (14 CPUs), 2026-10-18T13:33:16-02:00 → 2026-10-18T13:40:09-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p1-mif5000 | 1 | 20000 | 30000 | lag_growing | 83886.1 | 0 | 0 | 699 | 0.72 |  |
| p12 | 1 | 10000 | 20000 | p99_over_limit | 5308.4 | 0 | 0 | 1519 | 0.90 |  |

## p1-mif5000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | 27.3 | 4937 | 155.6 | sustained |
| 1 | 20000 | 19998 | 6.6 | 9713 | 66.6 | sustained |
| 2 | 30000 | 29999 | 18874.4 | 881486 | 28049.4 | lag_growing |
| 3 | 13333 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## p12

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -0.9 | 2502 | 819.2 | sustained |
| 1 | 20000 | 19999 | -1031.9 | 4882 | 5439.5 | p99_over_limit |
| 2 | 10000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
