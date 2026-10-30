# wave2-s5 (local)

Wave-2 S5 rule complexity (12 p, 1 KB, default auto in-flight): baseline, 5 / 20 set steps, identity JS (JS pool autoscale). 1-min steps (s5-explore method). Run once per ROUTER_ENGINE_CODEC; renamed wave2-s5-<codec>.json.


Run `stress-wave2-101812281426` on vps-c0215fee (14 CPUs), 2026-10-18T10:28:14-02:00 → 2026-10-18T10:55:08-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| baseline | 1 | 90000 | 110000 | p99_over_limit | 835.6 | 0 | 0 | 680 | 2.86 |  |
| s5 | 1 | 110000 | 130000 | lag_growing | 10485.8 | 0 | 0 | 621 | 4.55 |  |
| s20 | 1 | 72500 | 90000 | lag_growing | 11534.3 | 0 | 0 | 664 | 5.11 |  |
| js | 1 | 6500 | 8000 | lag_growing | 17039.4 | 0 | 0 | 721 | 5.92 |  |

## baseline

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | -0.1 | 7853 | 20.5 | sustained |
| 1 | 40000 | 39997 | -4.6 | 15438 | 32.3 | sustained |
| 2 | 57500 | 57496 | 61.0 | 22921 | 129.0 | sustained |
| 3 | 72500 | 72495 | 130.0 | 28803 | 170.0 | sustained |
| 4 | 90000 | 89989 | -43.0 | 35305 | 210.9 | sustained |
| 5 | 110000 | 109982 | -1337.7 | 43352 | 1212.4 | p99_over_limit |
| 6 | 43333 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## s5

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | 1.2 | 7896 | 20.7 | sustained |
| 1 | 40000 | 39997 | 3.5 | 15719 | 26.6 | sustained |
| 2 | 57500 | 57495 | -5.6 | 22523 | 141.3 | sustained |
| 3 | 72500 | 72466 | -8.3 | 27316 | 258.0 | sustained |
| 4 | 90000 | 89991 | 92.1 | 41067 | 190.5 | sustained |
| 5 | 110000 | 109988 | 31.0 | 44584 | 483.3 | sustained |
| 6 | 130000 | 129976 | 6880.5 | 392826 | 10485.8 | lag_growing |
| 7 | 160000 | 20920 | 0.0 | 0 | 0.0 | insufficient_samples |

## s20

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | -1.2 | 7679 | 20.7 | sustained |
| 1 | 40000 | 39997 | -8.2 | 15261 | 56.3 | sustained |
| 2 | 57500 | 57495 | 162.5 | 22257 | 565.2 | sustained |
| 3 | 72500 | 72494 | 95.5 | 32612 | 227.3 | sustained |
| 4 | 90000 | 89989 | 5446.7 | 328284 | 9699.3 | lag_growing |
| 5 | 110000 | 0 | 0.0 | 0 | 12451.8 | insufficient_samples |

## js

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 1000 | 1000 | 0.1 | 265 | 26.1 | sustained |
| 1 | 2000 | 2000 | 0.0 | 530 | 23.0 | sustained |
| 2 | 3000 | 3000 | 0.5 | 790 | 21.0 | sustained |
| 3 | 4000 | 4000 | -0.6 | 1069 | 25.6 | sustained |
| 4 | 5000 | 5000 | 0.5 | 1342 | 37.9 | sustained |
| 5 | 6500 | 6499 | -19.5 | 1796 | 598.0 | sustained |
| 6 | 8000 | 7999 | 1038.6 | 65823 | 16777.2 | lag_growing |
| 7 | 10000 | 0 | 0.0 | 0 | 20447.2 | insufficient_samples |
