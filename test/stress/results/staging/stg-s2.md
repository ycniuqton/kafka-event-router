# stg-s2 (staging)

Staging S2 short ramp - one route, 12 partitions, 1 KB, default maxInFlight, 1-min steps, stop on knee.

Run `stress-stg-101718532489` on vps-157a7542 (3 CPUs), 2026-10-17T16:53:24-02:00 → 2026-10-17T17:00:32-02:00.

| case | routes | max sustainable ev/s | knee ev/s | knee reason | p99 ms (all) | lost | dup | deploy p95 ms | worker CPU max | error |
|---|---|---|---|---|---|---|---|---|---|---|
| p12 | 1 | 10000 | 12500 | p99_over_limit | 11927.6 | 0 | 0 | 1483 | 0.99 |  |

## p12

| step | target | achieved | lag slope/s | lag end | p99 ms | verdict |
|---|---|---|---|---|---|---|
| 0 | 2000 | 2000 | -0.7 | 720 | 139.3 | sustained |
| 1 | 4000 | 4000 | 4.0 | 1623 | 95.2 | sustained |
| 2 | 6000 | 5998 | 6.5 | 2420 | 119.8 | sustained |
| 3 | 8000 | 7996 | 2.0 | 3046 | 153.6 | sustained |
| 4 | 10000 | 9993 | 85.2 | 6159 | 184.3 | sustained |
| 5 | 12500 | 12488 | -709.3 | 38631 | 10354.7 | p99_over_limit |
| 6 | 15000 | 13225 | 0.0 | 0 | 0.0 | insufficient_samples |
