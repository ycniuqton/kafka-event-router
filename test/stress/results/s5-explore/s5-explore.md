# s5-explore (local)

S5 exploration pass — rule complexity on one route (12 partitions, 1 KB, default maxInFlight 1000, i.e. the S2 p12 configuration whose knee is 65k ev/s). 1-min steps (20 s warm-up) locate each configuration's own knee; s5-confirm re-checks it with 5-min steps. Non-JS cases ramp 20k -> 90k, JS cases ramp 500 -> 10k (S1: ~0.8k ev/s per core).


Run `stress-s5-101712525175` on vps-c0215fee (14 CPUs), 2026-10-17T10:52:51-02:00 → 2026-10-17T12:05:35-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| baseline | 1 | 65000 | 72500 | lag_growing | 3407.9 | 0 | 0 | 655 | 4.03 |  |
| f1 | 1 | 65000 | 72500 | lag_growing | 3375.1 | 0 | 0 | 735 | 4.02 |  |
| f10 | 1 | 65000 | 72500 | p99_over_limit | 3768.3 | 0 | 0 | 632 | 4.11 |  |
| f50 | 1 | 65000 | 72500 | lag_growing | 5177.3 | 0 | 0 | 758 | 4.15 |  |
| s1 | 1 | 65000 | 72500 | lag_growing | 1458.2 | 0 | 0 | 675 | 4.07 |  |
| s5 | 1 | 65000 | 72500 | lag_growing | 9699.3 | 0 | 0 | 722 | 4.38 |  |
| s20 | 1 | 50000 | 57500 | lag_growing | 7733.2 | 0 | 0 | 637 | 4.82 |  |
| js | 1 | 5000 | 6500 | lag_growing | 18087.9 | 0 | 0 | 675 | 4.49 |  |
| f10-s5-js | 1 | 4000 | 5000 | p99_over_limit | 1015.8 | 0 | 0 | 760 | 4.52 |  |
| f50-s20-js | 1 | 4000 | 5000 | lag_growing | 14680.1 | 0 | 0 | 673 | 4.55 |  |

## baseline

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | 0.8 | 9274 | 24.3 | sustained |
| 1 | 30000 | 29998 | -2.6 | 13738 | 39.4 | sustained |
| 2 | 40000 | 39997 | 7.1 | 18413 | 86.0 | sustained |
| 3 | 50000 | 49996 | -20.4 | 23034 | 147.5 | sustained |
| 4 | 57500 | 57493 | 3.4 | 26151 | 174.1 | sustained |
| 5 | 65000 | 64992 | 48.6 | 30204 | 712.7 | sustained |
| 6 | 72500 | 72496 | 1104.5 | 88478 | 5767.2 | lag_growing |
| 7 | 80000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## f1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | -1.0 | 5321 | 21.2 | sustained |
| 1 | 30000 | 29999 | -4.3 | 7769 | 35.8 | sustained |
| 2 | 40000 | 39996 | 4.1 | 10563 | 75.8 | sustained |
| 3 | 50000 | 49995 | 6.8 | 13478 | 188.4 | sustained |
| 4 | 57500 | 57495 | -10.7 | 14930 | 413.7 | sustained |
| 5 | 65000 | 64994 | 400.9 | 17371 | 860.2 | sustained |
| 6 | 72500 | 72494 | 1308.3 | 75552 | 6160.4 | lag_growing |
| 7 | 80000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## f10

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | 1.4 | 8235 | 20.7 | sustained |
| 1 | 30000 | 29998 | 2.7 | 12481 | 27.6 | sustained |
| 2 | 40000 | 39996 | -3.1 | 16801 | 74.8 | sustained |
| 3 | 50000 | 49994 | -87.6 | 20960 | 139.3 | sustained |
| 4 | 57500 | 57493 | -51.1 | 23966 | 737.3 | sustained |
| 5 | 65000 | 64992 | 257.9 | 27273 | 770.0 | sustained |
| 6 | 72500 | 72493 | 721.1 | 71964 | 4980.7 | p99_over_limit |
| 7 | 40000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## f50

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | -2.0 | 10270 | 20.7 | sustained |
| 1 | 30000 | 29997 | -5.4 | 15391 | 57.3 | sustained |
| 2 | 40000 | 39998 | 2.7 | 20847 | 73.7 | sustained |
| 3 | 50000 | 49996 | 3.3 | 25883 | 47.6 | sustained |
| 4 | 57500 | 57496 | -42.2 | 30310 | 471.0 | sustained |
| 5 | 65000 | 64992 | 6.7 | 33639 | 409.6 | sustained |
| 6 | 72500 | 72495 | 952.7 | 122248 | 7012.4 | lag_growing |
| 7 | 40000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## s1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | 1.6 | 6952 | 21.0 | sustained |
| 1 | 30000 | 29998 | 1.8 | 10460 | 41.5 | sustained |
| 2 | 40000 | 39998 | 2.2 | 13883 | 213.0 | sustained |
| 3 | 50000 | 49996 | -4.1 | 17431 | 78.8 | sustained |
| 4 | 57500 | 57496 | -23.2 | 19997 | 446.5 | sustained |
| 5 | 65000 | 64994 | -38.9 | 22868 | 274.4 | sustained |
| 6 | 72500 | 72490 | 1072.9 | 58277 | 3342.3 | lag_growing |
| 7 | 40000 | 0 | 0.0 | 0 | 4194.3 | insufficient_samples |

## s5

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19999 | 1.0 | 4319 | 21.2 | sustained |
| 1 | 30000 | 29998 | -4.0 | 6644 | 36.4 | sustained |
| 2 | 40000 | 39997 | 0.0 | 8897 | 79.9 | sustained |
| 3 | 50000 | 49995 | 3.6 | 10808 | 172.0 | sustained |
| 4 | 57500 | 57496 | 14.6 | 17633 | 194.6 | sustained |
| 5 | 65000 | 64994 | -26.6 | 14280 | 393.2 | sustained |
| 6 | 72500 | 72495 | 4137.8 | 220515 | 11534.3 | lag_growing |
| 7 | 80000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## s20

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 20000 | 19998 | -7.8 | 7276 | 21.5 | sustained |
| 1 | 30000 | 29998 | 3.2 | 11004 | 28.4 | sustained |
| 2 | 40000 | 39998 | -5.7 | 14414 | 37.9 | sustained |
| 3 | 50000 | 49995 | -3.8 | 28074 | 372.7 | sustained |
| 4 | 57500 | 57495 | 724.3 | 88189 | 7733.2 | lag_growing |
| 5 | 32500 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## js

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 500 | 500 | -0.1 | 216 | 20.0 | sustained |
| 1 | 1000 | 1000 | 0.1 | 440 | 20.0 | sustained |
| 2 | 1500 | 1500 | 0.2 | 652 | 20.0 | sustained |
| 3 | 2000 | 2000 | -0.1 | 864 | 21.0 | sustained |
| 4 | 3000 | 3000 | 1.7 | 1297 | 21.0 | sustained |
| 5 | 4000 | 4000 | 0.8 | 1782 | 22.5 | sustained |
| 6 | 5000 | 5000 | -2.0 | 2165 | 139.3 | sustained |
| 7 | 6500 | 6500 | 966.9 | 59677 | 19136.5 | lag_growing |
| 8 | 4000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## f10-s5-js

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 500 | 500 | 0.1 | 128 | 20.0 | sustained |
| 1 | 1000 | 1000 | 0.4 | 248 | 20.2 | sustained |
| 2 | 1500 | 1500 | 0.1 | 369 | 20.5 | sustained |
| 3 | 2000 | 2000 | -0.8 | 488 | 20.5 | sustained |
| 4 | 3000 | 3000 | -0.5 | 733 | 21.0 | sustained |
| 5 | 4000 | 4000 | 0.0 | 1008 | 23.3 | sustained |
| 6 | 5000 | 4999 | -5.7 | 1420 | 1572.9 | p99_over_limit |
| 7 | 3250 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## f50-s20-js

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 500 | 500 | 0.1 | 216 | 20.0 | sustained |
| 1 | 1000 | 1000 | 0.1 | 424 | 20.2 | sustained |
| 2 | 1500 | 1500 | -0.2 | 623 | 20.2 | sustained |
| 3 | 2000 | 2000 | 0.2 | 848 | 20.7 | sustained |
| 4 | 3000 | 3000 | 0.2 | 1291 | 22.8 | sustained |
| 5 | 4000 | 4000 | 1.0 | 1757 | 94.2 | sustained |
| 6 | 5000 | 4999 | 541.3 | 33960 | 16384.0 | lag_growing |
| 7 | 3250 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
