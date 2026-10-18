# broker-baseline (2026-10-17)

Local Kafka 4.4.0 KRaft on 127.0.0.1:39092, pinned to cores 6-9, heap 4g; clients on other cores.
1 KB records, acks=all, 12 partitions, RF 1, no compression.

- Produce ceiling: ~347k rec/s (339 MB/s) with 3 parallel producers, but p99 3.8-3.9 s (saturated).
- Single producer: 138k rec/s (defaults), 177k rec/s (linger 10 ms, 256 KiB batches); p99 0.4-1.1 s.
- Low-latency point: 100k rec/s throttled -> p99 45 ms, p99.9 148 ms.
- Consume ceiling: 247k rec/s single consumer, ~560k rec/s with 3 consumers (page-cache hot).
- Bottleneck is the VM disk (O_DIRECT write 405 MB/s); the broker used ~1.05 of its 4 cores.
- For the router: one route consumes and produces on this same broker, so treat ~150-170k ev/s as the hard ceiling
  and ~100k ev/s as the point where Kafka itself adds no significant latency.
