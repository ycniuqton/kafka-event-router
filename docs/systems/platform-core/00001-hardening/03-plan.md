# platform-core — Plan (00001-hardening)

Incremental version on top of `00000-init`. **Unchanged:** every item PC-xx in `00000-init/03-plan.md` keeps its
state there. Mark the step you start `[~]`, finished `[x]`; never delete items.

## BE
- [x] PC-HD-01 `store.ListenerHub` (dedicated multiplexed LISTEN connection, fan-out, reset notices, backoff, keepalive) + `ListenConn`/`ListenDialer` bases, `PgListenDialer` — `internal/store/listener_hub.go`, `listen_conn.go`
- [x] PC-HD-02 Unit tests with a fake connection — `internal/store/listener_hub_test.go`, `internal/runtime/changes/listener_test.go`
- [x] PC-HD-03 `DB.Notifications()` owns the hub (started on first subscribe, stopped by `Close`); `DB.Check` on a dedicated health connection — `internal/store/db.go`, `health_probe.go`
- [x] PC-HD-04 Pool size knob `ROUTER_DB_POOL_MAX_CONNS` + sane default — `internal/platform/config/{keys,config,loader}.go` (shared, small), `internal/store/db.go`
- [x] PC-HD-05 Move `changes.PgListener` and `rbac.InvalidationListener` to the hub — `internal/runtime/changes/broker.go`, `internal/rbac/invalidation.go`, `cmd/api/wire_{runtime,monitoring_alerts,routes_p3,access}.go` (one-line wiring edits)
- [x] PC-HD-06 `scheduler.PgLeaderElector` winner hijacks its connection out of the pool — `internal/scheduler/leader.go`
- [x] PC-HD-07 Docs: design (this folder), staging README note on `pool_max_conns` — `deploy/staging/README.md`

## Verification (orchestrator, after merge)
- [x] PC-HD-08 Integration: rbac invalidation IT, leader failover IT (`ROUTER_IT_DATABASE_URL`), api `/readyz` under load with default pool size — done (wave 2): `TestIntegrationNotifyInvalidatesTwoManagers`, `TestPgLeaderElectionSingleLeaderAndFailover`, ListenerHub ITs pass on router_dev; `/readyz` 292/292 × 200 (p99 1.6 ms) under 48 concurrent DB reads; but the reads timed out at 10 s through the dev tunnel and worker heartbeat/status calls got 500/503 (shared request pool), and the shared Postgres went into crash recovery for ~1 s at the end of the probe — see the wave-2 report open issues
