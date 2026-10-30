# wave2-s2 (local)

Wave-2 S2 (fast codec + adaptive in-flight + memory budget): one route, 1 KB, 1-min steps (20 s warm-up, s2-explore method). Cases p1/p12 with the default (auto) in-flight limit and with maxInFlight 5000 (auto, cap 5000). Run once per ROUTER_ENGINE_CODEC (fast, tree); results are renamed wave2-s2-<codec>.json.


Run `stress-wave2-101812551814` on vps-c0215fee (14 CPUs), 2026-10-18T10:55:18-02:00 → 2026-10-18T11:21:58-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p1 | 1 | 30000 | 40000 | lag_growing | 17039.4 | 0 | 0 | 666 | 1.79 |  |
| p1-mif5000 | 1 | 30000 | 40000 | lag_growing | 18350.1 | 0 | 0 | 696 | 1.82 |  |
| p12 | 1 | 80000 | 100000 | lag_growing | 24903.7 | 0 | 0 | 760 | 5.04 |  |
| p12-mif5000 | 1 | 80000 | 100000 | lag_growing | 23593.0 | 0 | 0 | 778 | 5.05 |  |

## p1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | 1.5 | 3812 | 18.4 | sustained |
| 1 | 20000 | 19999 | 1.3 | 7445 | 22.3 | sustained |
| 2 | 30000 | 29998 | 34.0 | 11268 | 364.5 | sustained |
| 3 | 40000 | 39997 | 9519.8 | 567157 | 13893.6 | lag_growing |
| 4 | 16667 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## p1-mif5000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -0.1 | 2577 | 18.4 | sustained |
| 1 | 20000 | 19999 | -2.5 | 5036 | 20.5 | sustained |
| 2 | 30000 | 29998 | -8.3 | 9827 | 335.9 | sustained |
| 3 | 40000 | 39997 | 9447.5 | 586407 | 14417.9 | lag_growing |
| 4 | 50000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## p12

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | 1.1 | 2807 | 19.5 | sustained |
| 1 | 20000 | 19998 | 2.2 | 5413 | 20.7 | sustained |
| 2 | 30000 | 29996 | -1.2 | 8286 | 26.6 | sustained |
| 3 | 40000 | 39997 | 7.3 | 11132 | 65.5 | sustained |
| 4 | 50000 | 49996 | 10.1 | 13942 | 135.2 | sustained |
| 5 | 65000 | 64994 | 41.3 | 18647 | 143.4 | sustained |
| 6 | 80000 | 79993 | 146.3 | 22972 | 397.3 | sustained |
| 7 | 100000 | 99986 | 13072.4 | 779657 | 18087.9 | lag_growing |
| 8 | 80000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## p12-mif5000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -0.9 | 4232 | 19.7 | sustained |
| 1 | 20000 | 19999 | -0.5 | 8557 | 21.8 | sustained |
| 2 | 30000 | 29997 | -7.3 | 12738 | 23.6 | sustained |
| 3 | 40000 | 39999 | 4.8 | 17345 | 56.8 | sustained |
| 4 | 50000 | 49994 | 18.1 | 21405 | 112.6 | sustained |
| 5 | 65000 | 64993 | 15.7 | 28101 | 352.3 | sustained |
| 6 | 80000 | 79992 | 266.4 | 35188 | 827.4 | sustained |
| 7 | 100000 | 99994 | 11016.7 | 742925 | 16515.1 | lag_growing |
| 8 | 120000 | 23521 | 0.0 | 0 | 0.0 | insufficient_samples |
