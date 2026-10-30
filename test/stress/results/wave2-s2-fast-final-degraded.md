# wave2-s2 (local)

Wave-2 S2 (fast codec + adaptive in-flight + memory budget): one route, 1 KB, 1-min steps (20 s warm-up, s2-explore method). Cases p1/p12 with the default (auto) in-flight limit and with maxInFlight 5000 (auto, cap 5000). Run once per ROUTER_ENGINE_CODEC (fast, tree); results are renamed wave2-s2-<codec>.json.


Run `stress-wave2-101816574067` on vps-c0215fee (14 CPUs), 2026-10-18T14:57:40-02:00 → 2026-10-18T15:19:47-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p1 | 1 | 40000 | 50000 | lag_growing | 3768.3 | 0 | 0 | 841 | 1.79 |  |
| p1-mif5000 | 1 | 65000 | 80000 | lag_growing | 2162.7 | 0 | 0 | 732 | 1.97 |  |
| p12 | 1 | 40000 | 50000 | p99_over_limit | 1441.8 | 0 | 0 | 707 | 1.69 |  |
| p12-mif5000 | 1 | 30000 | 40000 | p99_over_limit | 1720.3 | 0 | 0 | 2080 | 1.39 |  |

## p1

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | 0.4 | 2794 | 18.4 | sustained |
| 1 | 20000 | 19998 | -3.3 | 5644 | 19.7 | sustained |
| 2 | 30000 | 29997 | -71.5 | 9631 | 933.9 | sustained |
| 3 | 40000 | 39997 | 59.0 | 11009 | 901.1 | sustained |
| 4 | 50000 | 49992 | 3214.4 | 104121 | 1900.5 | lag_growing |
| 5 | 65000 | 0 | 0.0 | 0 | 3768.3 | insufficient_samples |

## p1-mif5000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -1.6 | 2608 | 45.1 | sustained |
| 1 | 20000 | 19999 | 58.1 | 5320 | 901.1 | sustained |
| 2 | 30000 | 29997 | 1.5 | 7827 | 35.8 | sustained |
| 3 | 40000 | 39997 | 4.2 | 10584 | 31.5 | sustained |
| 4 | 50000 | 49996 | 15.6 | 13541 | 91.1 | sustained |
| 5 | 65000 | 64996 | 22.4 | 17293 | 235.5 | sustained |
| 6 | 80000 | 79992 | 2886.2 | 195729 | 2195.5 | lag_growing |
| 7 | 33333 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## p12

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -1.7 | 3538 | 22.5 | sustained |
| 1 | 20000 | 19999 | -8.8 | 7003 | 458.8 | sustained |
| 2 | 30000 | 29997 | 18.5 | 11619 | 581.6 | sustained |
| 3 | 40000 | 39996 | 20.0 | 14188 | 933.9 | sustained |
| 4 | 50000 | 49996 | -210.6 | 18064 | 1802.2 | p99_over_limit |
| 5 | 21667 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |

## p12-mif5000

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 10000 | 9999 | -0.7 | 3531 | 251.9 | sustained |
| 1 | 20000 | 19998 | -35.7 | 6965 | 974.8 | sustained |
| 2 | 30000 | 29997 | -123.0 | 10595 | 413.7 | sustained |
| 3 | 40000 | 39998 | 177.9 | 13767 | 2064.4 | p99_over_limit |
| 4 | 16667 | 0 | 0.0 | 0 | 0.0 | insufficient_samples |
