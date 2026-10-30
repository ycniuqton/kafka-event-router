# wave2-s5 (local)

Wave-2 S5 rule complexity (12 p, 1 KB, default auto in-flight): baseline, 5 / 20 set steps, identity JS (JS pool autoscale). 1-min steps (s5-explore method). Run once per ROUTER_ENGINE_CODEC; renamed wave2-s5-<codec>.json.


Run `stress-wave2-101817194711` on vps-c0215fee (14 CPUs), 2026-10-18T15:19:47-02:00 → 2026-10-18T15:29:53-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| baseline | 1 | 20000 | 40000 | lag_growing | 1540.1 | 0 | 0 | 1898 | 1.36 |  |
| s5 | 1 | 40000 | 57500 | lag_growing | 2850.8 | 0 | 0 | 1967 | 2.72 |  |
| s20 | 1 | 40000 | 57500 | lag_growing | 5046.3 | 0 | 0 | 2717 | 4.58 |  |
| js | 1 | 1000 | 0 |  | 26.4 | 0 | 0 | 2138 | 1.12 | context canceled |

## baseline

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19999 | 0.5 | 4379 | 688.1 | sustained |
| 1 | 40000 | 39997 | 1187.0 | 49886 | 1540.1 | lag_growing |
| 2 | 57500 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## s5

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | 28.0 | 5148 | 282.6 | sustained |
| 1 | 40000 | 39939 | -139.5 | 8735 | 909.3 | sustained |
| 2 | 57500 | 57495 | 2578.9 | 89020 | 2228.2 | lag_growing |
| 3 | 24167 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## s20

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19999 | -0.9 | 10604 | 303.1 | sustained |
| 1 | 40000 | 39996 | 124.9 | 21252 | 884.7 | sustained |
| 2 | 57500 | 57497 | 2866.2 | 179740 | 4849.7 | lag_growing |
| 3 | 24167 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## js

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 1000 | 1000 | -0.6 | 336 | 25.6 | sustained |
