# wave2-s2 (local)

Wave-2 S2 (fast codec + adaptive in-flight + memory budget): one route, 1 KB, 1-min steps (20 s warm-up, s2-explore method). Cases p1/p12 with the default (auto) in-flight limit and with maxInFlight 5000 (auto, cap 5000). Run once per ROUTER_ENGINE_CODEC (fast, tree); results are renamed wave2-s2-<codec>.json.


Run `stress-wave2-101814244647` on vps-c0215fee (14 CPUs), 2026-10-18T12:24:46-02:00 → 2026-10-18T12:58:27-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p1 | 1 | 80000 | 100000 | lag_growing | 39845.9 | 0 | 0 | 693 | 1.84 |  |
| p1-mif5000 | 1 | 65000 | 80000 | lag_growing | 1441.8 | 0 | 0 | 773 | 1.73 |  |
| p12 | 1 | 50000 | 65000 | lag_growing | 819.2 | 0 | 0 | 884 | 1.87 |  |
| p12-mif5000 | 1 | 140000 | 160000 | lag_growing | 1343.5 | 0 | 0 | 1466 | 3.94 |  |

## p1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -0.9 | 1929 | 22.3 | sustained |
| 1 | 20000 | 19998 | -3.3 | 3823 | 19.5 | sustained |
| 2 | 30000 | 29998 | -1.3 | 5804 | 22.0 | sustained |
| 3 | 40000 | 39998 | -0.0 | 7833 | 137.2 | sustained |
| 4 | 50000 | 49996 | -59.9 | 10110 | 161.8 | sustained |
| 5 | 65000 | 64994 | -104.6 | 13145 | 573.4 | sustained |
| 6 | 80000 | 79996 | 76.9 | 19348 | 202.8 | sustained |
| 7 | 100000 | 99988 | 41480.6 | 2492232 | 24641.5 | lag_growing |
| 8 | 80000 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## p1-mif5000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -0.6 | 2228 | 18.4 | sustained |
| 1 | 20000 | 19999 | -0.5 | 4282 | 20.0 | sustained |
| 2 | 30000 | 29998 | 0.9 | 6509 | 106.5 | sustained |
| 3 | 40000 | 39997 | -6.2 | 8592 | 69.6 | sustained |
| 4 | 50000 | 49995 | -23.6 | 11636 | 239.6 | sustained |
| 5 | 65000 | 64993 | -21.4 | 13850 | 426.0 | sustained |
| 6 | 80000 | 79991 | 1427.7 | 123017 | 1490.9 | lag_growing |
| 7 | 33333 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## p12

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | 3.8 | 2712 | 19.7 | sustained |
| 1 | 20000 | 19998 | 4.2 | 5155 | 21.0 | sustained |
| 2 | 30000 | 29998 | -4.1 | 7559 | 34.8 | sustained |
| 3 | 40000 | 39996 | 1.5 | 10629 | 73.7 | sustained |
| 4 | 50000 | 49995 | -53.2 | 13053 | 78.8 | sustained |
| 5 | 65000 | 64689 | 719.6 | 61923 | 1024.0 | lag_growing |
| 6 | 53333 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## p12-mif5000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | 4.8 | 2376 | 153.6 | sustained |
| 1 | 20000 | 19998 | 2.6 | 4777 | 21.5 | sustained |
| 2 | 30000 | 29997 | 0.1 | 7128 | 50.2 | sustained |
| 3 | 40000 | 39996 | 7.3 | 9539 | 53.8 | sustained |
| 4 | 50000 | 49994 | 1.8 | 12087 | 71.7 | sustained |
| 5 | 65000 | 64996 | -5.6 | 15895 | 107.5 | sustained |
| 6 | 80000 | 79989 | 7.5 | 19520 | 58.4 | sustained |
| 7 | 100000 | 99989 | 54.4 | 24656 | 311.3 | sustained |
| 8 | 120000 | 119984 | -20.6 | 29247 | 323.6 | sustained |
| 9 | 140000 | 139980 | 45.1 | 37786 | 524.3 | sustained |
| 10 | 160000 | 159973 | 1955.6 | 111569 | 2490.4 | lag_growing |
| 11 | 133333 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
