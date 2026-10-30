# wave2-s5 (local)

Wave-2 S5 rule complexity (12 p, 1 KB, default auto in-flight): baseline, 5 / 20 set steps, identity JS (JS pool autoscale). 1-min steps (s5-explore method). Run once per ROUTER_ENGINE_CODEC; renamed wave2-s5-<codec>.json.


Run `stress-wave2-101813215895` on vps-c0215fee (14 CPUs), 2026-10-18T11:21:58-02:00 → 2026-10-18T11:42:33-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| baseline | 1 | 72500 | 90000 | lag_growing | 8192.0 | 0 | 0 | 752 | 4.98 |  |
| s5 | 1 | 57500 | 72500 | p99_over_limit | 1998.8 | 0 | 0 | 903 | 5.07 |  |
| s20 | 1 | 40000 | 57500 | lag_growing | 7602.2 | 0 | 0 | 865 | 5.35 |  |
| js | 1 | 6500 | 8000 | lag_growing | 18087.9 | 0 | 0 | 1788 | 5.92 |  |

## baseline

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19997 | 5.3 | 5759 | 20.7 | sustained |
| 1 | 40000 | 39997 | -1.1 | 11429 | 38.4 | sustained |
| 2 | 57500 | 57494 | -3.7 | 16327 | 163.8 | sustained |
| 3 | 72500 | 72495 | -142.9 | 21067 | 130.0 | sustained |
| 4 | 90000 | 89989 | 3096.2 | 255155 | 8126.5 | lag_growing |
| 5 | 36667 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## s5

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | 1.3 | 2285 | 25.6 | sustained |
| 1 | 40000 | 39996 | -3.9 | 4698 | 39.9 | sustained |
| 2 | 57500 | 57495 | -28.0 | 6867 | 630.8 | sustained |
| 3 | 72500 | 72489 | 221.1 | 37742 | 2326.5 | p99_over_limit |
| 4 | 90000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## s20

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | 0.3 | 5096 | 21.8 | sustained |
| 1 | 40000 | 39997 | 5.9 | 10358 | 66.6 | sustained |
| 2 | 57500 | 57497 | 1341.2 | 107755 | 5701.6 | lag_growing |
| 3 | 72500 | 30636 | 0.0 | 0 | 7798.8 | insufficient_samples |

## js

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 1000 | 1000 | 0.0 | 264 | 21.0 | sustained |
| 1 | 2000 | 2000 | 0.0 | 532 | 21.2 | sustained |
| 2 | 3000 | 3000 | 0.6 | 776 | 21.2 | sustained |
| 3 | 4000 | 3999 | 0.6 | 1036 | 24.6 | sustained |
| 4 | 5000 | 4999 | 1.7 | 1320 | 34.8 | sustained |
| 5 | 6500 | 6499 | 29.9 | 3525 | 786.4 | sustained |
| 6 | 8000 | 7999 | 1073.9 | 68362 | 18350.1 | lag_growing |
| 7 | 6667 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
