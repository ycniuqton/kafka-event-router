# s5-confirm (local)

S5 confirmation pass — 5-min steps at the max sustainable rate found by s5-explore (and the next rate where the explore knee differed from the baseline), full criteria (30 s warm-up). 12 partitions, 1 KB, default maxInFlight 1000. f1/f10/s1 share the baseline knee (65k, in-flight bound) and are represented by their heaviest sibling (f50, s5); the baseline itself is S2 p12 (65k confirmed).


Run `stress-s5-101714295895` on vps-c0215fee (14 CPUs), 2026-10-17T12:29:58-02:00 → 2026-10-17T13:24:35-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| f50 | 1 | 65000 | 0 |  | 622.6 | 0 | 0 | 815 | 4.31 |  |
| s5 | 1 | 0 | 65000 | p99_over_limit | 1278.0 | 0 | 0 | 628 | 4.44 |  |
| s20 | 1 | 50000 | 57500 | lag_growing | 47710.2 | 0 | 0 | 624 | 4.82 |  |
| js | 1 | 5000 | 6500 | lag_growing | 101711.9 | 0 | 0 | 758 | 4.51 |  |
| f10-s5-js | 1 | 4000 | 5000 | p99_over_limit | 17563.6 | 0 | 0 | 668 | 4.55 |  |
| f50-s20-js | 1 | 4000 | 5000 | lag_growing | 92274.7 | 0 | 0 | 684 | 4.58 |  |

## f50

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 65000 | 64999 | -8.4 | 22000 | 630.8 | sustained |

## s5

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 65000 | 64999 | 35.1 | 21313 | 1294.3 | p99_over_limit |

## s20

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 50000 | 49999 | -10.5 | 20214 | 647.2 | sustained |
| 1 | 57500 | 57499 | 1151.6 | 398322 | 42991.6 | lag_growing |

## js

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 5000 | 5000 | 0.2 | 623 | 188.4 | sustained |
| 1 | 6500 | 6500 | 966.0 | 291894 | 95420.4 | lag_growing |

## f10-s5-js

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 4000 | 4000 | 0.0 | 1065 | 25.9 | sustained |
| 1 | 5000 | 5000 | 46.9 | 12831 | 19136.5 | p99_over_limit |

## f50-s20-js

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 4000 | 4000 | 0.3 | 1666 | 434.2 | sustained |
| 1 | 5000 | 5000 | 631.8 | 192213 | 87031.8 | lag_growing |
