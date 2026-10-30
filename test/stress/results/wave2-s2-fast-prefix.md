# wave2-s2 (local)

Wave-2 S2 (fast codec + adaptive in-flight + memory budget): one route, 1 KB, 1-min steps (20 s warm-up, s2-explore method). Cases p1/p12 with the default (auto) in-flight limit and with maxInFlight 5000 (auto, cap 5000). Run once per ROUTER_ENGINE_CODEC (fast, tree); results are renamed wave2-s2-<codec>.json.


Run `stress-wave2-101811532415` on vps-c0215fee (14 CPUs), 2026-10-18T09:53:24-02:00 → 2026-10-18T10:28:14-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p1 | 1 | 20000 | 30000 | lag_growing | 9699.3 | 0 | 0 | 653 | 0.63 |  |
| p1-mif5000 | 1 | 80000 | 100000 | lag_growing | 13762.6 | 0 | 0 | 668 | 1.89 |  |
| p12 | 1 | 140000 | 160000 | p99_over_limit | 4161.5 | 0 | 0 | 733 | 3.93 |  |
| p12-mif5000 | 1 | 140000 | 160000 | p99_over_limit | 4325.4 | 0 | 0 | 703 | 3.92 |  |

## p1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -1.0 | 3204 | 19.2 | sustained |
| 1 | 20000 | 19998 | -3.0 | 6564 | 704.5 | sustained |
| 2 | 30000 | 29997 | 4210.6 | 264422 | 8519.7 | lag_growing |
| 3 | 13333 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## p1-mif5000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -0.3 | 2130 | 18.2 | sustained |
| 1 | 20000 | 19998 | 0.4 | 4333 | 32.8 | sustained |
| 2 | 30000 | 29997 | 2.3 | 6627 | 135.2 | sustained |
| 3 | 40000 | 39997 | -2.5 | 8633 | 23.0 | sustained |
| 4 | 50000 | 49996 | -3.0 | 10717 | 69.6 | sustained |
| 5 | 65000 | 64993 | -34.7 | 14430 | 206.8 | sustained |
| 6 | 80000 | 79992 | 1.4 | 18055 | 258.0 | sustained |
| 7 | 100000 | 99993 | 20384.5 | 1201491 | 11796.5 | lag_growing |
| 8 | 40000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## p12

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -1.3 | 2721 | 19.7 | sustained |
| 1 | 20000 | 19999 | 0.5 | 5728 | 23.0 | sustained |
| 2 | 30000 | 29998 | 1.0 | 8278 | 27.6 | sustained |
| 3 | 40000 | 39996 | 2.3 | 11097 | 69.6 | sustained |
| 4 | 50000 | 49996 | 0.3 | 13723 | 40.4 | sustained |
| 5 | 65000 | 64995 | 89.6 | 24048 | 163.8 | sustained |
| 6 | 80000 | 79990 | -6.2 | 22091 | 254.0 | sustained |
| 7 | 100000 | 99990 | 14.1 | 27829 | 188.4 | sustained |
| 8 | 120000 | 119987 | 75.6 | 34107 | 356.4 | sustained |
| 9 | 140000 | 139979 | 189.2 | 39567 | 401.4 | sustained |
| 10 | 160000 | 159966 | 919.3 | 139941 | 4980.7 | p99_over_limit |
| 11 | 66667 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## p12-mif5000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | 5.6 | 2409 | 19.5 | sustained |
| 1 | 20000 | 19998 | 1.2 | 4729 | 23.6 | sustained |
| 2 | 30000 | 29997 | 2.0 | 7228 | 21.2 | sustained |
| 3 | 40000 | 39997 | 3.3 | 9574 | 66.6 | sustained |
| 4 | 50000 | 49996 | 7.6 | 12043 | 26.9 | sustained |
| 5 | 65000 | 64995 | 102.9 | 15873 | 109.6 | sustained |
| 6 | 80000 | 79990 | 19.8 | 19452 | 139.3 | sustained |
| 7 | 100000 | 99990 | 1.5 | 24403 | 145.4 | sustained |
| 8 | 120000 | 119984 | -40.0 | 28822 | 315.4 | sustained |
| 9 | 140000 | 139976 | -83.1 | 35647 | 434.2 | sustained |
| 10 | 160000 | 159972 | 1283.8 | 170413 | 6422.5 | p99_over_limit |
| 11 | 66667 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
