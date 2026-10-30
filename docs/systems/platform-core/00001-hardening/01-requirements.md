# platform-core — Requirements (00001-hardening)

Incremental version on top of `00000-init`. **Unchanged:** every requirement FR-01..FR-xx and NFR of
`00000-init/01-requirements.md` still applies as written. This version only adds the database connection
hardening found during the route-runtime stress test (staging readiness ping timeouts).

## Business intent
The API keeps several long-lived Postgres sessions (LISTEN for runtime changes, RBAC invalidation; advisory
locks for leader election). They were taken from the shared pgx pool, which pgxpool caps at max(4, CPUs): on a
small host every pooled connection was parked in LISTEN, requests queued and `/readyz` timed out. Staging
worked around it with `pool_max_conns=16` in the DSN. Long-lived sessions must never compete with requests.

## Functional requirements
- HD-01 Every LISTEN/NOTIFY consumer of a process shares **one dedicated listener connection** (not from the
  pool), owned by `store.DB`; the connection LISTENs to the union of subscribed channels and fans each
  notification out in-process to the subscribers of its channel.
- HD-02 The listener reconnects with exponential backoff (0.5 s → 30 s), re-LISTENs every subscribed channel
  and then sends every subscriber a **reset** notice ("notifications may have been missed — resync").
- HD-03 Subscribing to a new channel while the listener is running takes effect without a reconnect.
- HD-04 A slow subscriber never blocks the listener or other subscribers; when its buffer is full its pending
  notices are coalesced into one reset notice (no silent loss: the subscriber resyncs).
- HD-05 The listener pings its idle connection periodically (30 s) so a dead session is detected and replaced.
- HD-06 Existing consumers move to the shared listener with unchanged behaviour: route-runtime change hints
  (`router_runtime`, resync hint on reset), RBAC invalidation (`rbac_changed`, invalidate-all on reset).
- HD-07 Leader electors (scheduler, secret rotation, DLQ janitor) try the advisory lock on a pooled connection;
  the winner takes that connection out of the pool (`Hijack`), so a held lock never occupies a pool slot.
- HD-08 The readiness check pings Postgres on its own dedicated connection, so a busy pool never fails `/readyz`.
- HD-09 Pool size: `ROUTER_DB_POOL_MAX_CONNS` (optional) sets the pool's max connections; default
  max(10, 2 × CPUs); a `pool_max_conns` DSN parameter still wins. The staging DSN workaround is no longer needed.
- HD-10 Connection budget per process is documented: pool + 1 listener + 1 health + 1 per held leader lock.

## Non-functional
- No new goroutine per subscriber in the listener; fan-out cost O(subscribers of the channel).
- Unit tests use a fake connection (no Postgres): fan-out, dynamic subscribe, reconnect + resubscribe + reset,
  overflow coalescing, keepalive ping failure.

## Out of scope
- Moving short-lived advisory locks (migrator 7001) off the pool; pgbouncer.
