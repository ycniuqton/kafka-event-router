# Staging stress pass (2026-10-17, 16:53-17:50 -02)

Host 3 vCPU / 5.8 GB / no swap; worker container 1 CPU / 512 MB (build of 2026-10-16, without the backlog and
heartbeat fixes); Kafka 1 CPU / 1 GB, single node, shared. stressrun + loadgen ran on the host (127.0.0.1:29092),
target `targets-staging-host.yaml`, envTag dev (no approval, 4-eyes untouched). Stress topics had bounded retention.

- S2 p12 1 KB: max 10k ev/s (p99 184 ms), knee 12.5k (p99 10.4 s). Worker CPU 0.81 avg / 0.91 max of its 1.0
  limit at 10k, 0.98 at 12.5k; Kafka 0.5. First limit: worker container CPU quota (~11-12k ev/s per core).
- S3 10/50/80 routes at 10 ev/s/route: r10, r50 sustained; r80 p99 3.1 s from a 4 s spike in the first ~45 s after
  30 new routes started, then 41-135 ms. ~13 goroutines and ~0.3 MB per route; Kafka ~7 m-cores per route.
  Stopped at 80: scheduler capacity 100 units (1.1 per 1-partition route, orders-smoke 1.3) = ~89 routes.
- S8 40 min at 7k (70 %): 16.8M events, 0 lost, 0 dup, p99 926 ms overall; four 15-s windows of 1.3-2.7 s about
  every 8 min, with Kafka at its 1.0 CPU limit (cause not proven). RSS 70-139 MB, goroutines 63-75, no growth.
- Host: memory max 34 %, disk max 51 % (Kafka volume max 8.0 GB), PG connections 33-41, 0 unhealthy samples.
- orders-smoke pair check passed before, between S3 and S8, and after.
