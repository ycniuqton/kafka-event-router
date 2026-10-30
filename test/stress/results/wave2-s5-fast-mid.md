# wave2-s5 (local)

Wave-2 S5 rule complexity (12 p, 1 KB, default auto in-flight): baseline, 5 / 20 set steps, identity JS (JS pool autoscale). 1-min steps (s5-explore method). Run once per ROUTER_ENGINE_CODEC; renamed wave2-s5-<codec>.json.


Run `stress-wave2-101814582746` on vps-c0215fee (14 CPUs), 2026-10-18T12:58:27-02:00 → 2026-10-18T13:24:05-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| baseline | 1 | 160000 | 0 |  | 729.1 | 0 | 0 | 3169 | 3.86 |  |
| s5 | 1 | 90000 | 110000 | p99_over_limit | 1343.5 | 0 | 0 | 633 | 4.32 |  |
| s20 | 1 | 57500 | 72500 | lag_growing | 28835.8 | 0 | 0 | 773 | 4.76 |  |
| js | 1 | 5000 | 6500 | p99_over_limit | 1703.9 | 0 | 0 | 1146 | 5.87 |  |

## baseline

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19999 | 0.5 | 5069 | 20.7 | sustained |
| 1 | 40000 | 39998 | -4.2 | 10024 | 48.6 | sustained |
| 2 | 57500 | 57495 | 6.3 | 14705 | 126.0 | sustained |
| 3 | 72500 | 72494 | 52.3 | 18500 | 78.8 | sustained |
| 4 | 90000 | 89993 | 15.0 | 23042 | 241.7 | sustained |
| 5 | 110000 | 109991 | -637.1 | 28059 | 835.6 | sustained |
| 6 | 130000 | 129983 | -22.0 | 33568 | 512.0 | sustained |
| 7 | 160000 | 159979 | 1074.9 | 98469 | 950.3 | sustained |

## s5

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | -0.9 | 7944 | 20.5 | sustained |
| 1 | 40000 | 39997 | 6.3 | 16221 | 38.4 | sustained |
| 2 | 57500 | 57495 | -15.5 | 22773 | 109.6 | sustained |
| 3 | 72500 | 72493 | -8.9 | 29446 | 167.9 | sustained |
| 4 | 90000 | 89991 | -34.5 | 35826 | 256.0 | sustained |
| 5 | 110000 | 109986 | -2660.2 | 44801 | 1785.9 | p99_over_limit |
| 6 | 130000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## s20

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | 1.7 | 5899 | 21.0 | sustained |
| 1 | 40000 | 39996 | -4.9 | 11832 | 50.7 | sustained |
| 2 | 57500 | 57496 | 33.9 | 17606 | 282.6 | sustained |
| 3 | 72500 | 72323 | 24269.7 | 818724 | 11272.2 | lag_growing |
| 4 | 90000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## js

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 1000 | 1000 | 0.1 | 276 | 23.3 | sustained |
| 1 | 2000 | 2000 | 0.4 | 545 | 23.8 | sustained |
| 2 | 3000 | 2999 | -0.3 | 792 | 30.2 | sustained |
| 3 | 4000 | 4000 | 2.3 | 1063 | 141.3 | sustained |
| 4 | 5000 | 4999 | -0.7 | 1326 | 516.1 | sustained |
| 5 | 6500 | 6499 | -43.4 | 2329 | 2392.1 | p99_over_limit |
| 6 | 2667 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
