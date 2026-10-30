# platform-core — Design (00001-hardening)

Incremental version on top of `00000-init`. **Unchanged:** everything in `00000-init/02-design.md` (packages,
migrations, conventions, `store.Notifier` / `pg_notify` publishing, channel names). This version changes only
*how a process holds its long-lived Postgres sessions*.

## 1. Problem

```
api process (before)                      pgxpool (max(4, CPUs) conns)
  changes.PgListener ×3  ── Acquire ──→   conn 1  LISTEN router_runtime   (forever)
  rbac.InvalidationListener ─ Acquire ─→  conn 2  LISTEN router_runtime   (forever)
  PgLeaderElector ×3 (winner) ─ Acquire ─→ conn 3  LISTEN router_runtime   (forever)
                                           conn 4  LISTEN rbac_changed     (forever)
  HTTP handlers / readiness ping ─ Acquire → (wait …)  → /readyz timeout on a 2-CPU host
```

The api starts three router_runtime listeners (runtime SSE + scheduler, monitoring push, replay relay), one
rbac listener and up to three advisory-lock holders (scheduler, secret rotation, DLQ janitor); each kept a
pooled connection forever. Staging set `pool_max_conns=16` in the DSN as a workaround.

## 2. Connection layout (after)

| Session | Owner | Count per process | From the pool? |
|---|---|---|---|
| Statements, transactions | `store.DB.Q` / `TxManager` | `MaxConns` (§4) | yes |
| LISTEN (all channels) | `store.ListenerHub` (`DB.Notifications()`) | 1, dialled on first subscribe | no (`SessionDialer`) |
| Readiness ping | `store.HealthProbe` (`DB.Check`) | 1, dialled on first check | no (`SessionDialer`) |
| Held advisory lock | `scheduler.PgLeaderElector` winner | 1 per held lock | taken from the pool for the try, then `Hijack()`ed out |
| Migration lock 7001 | `store.Migrator` | short-lived | yes (unchanged) |

`SessionDialer` (`store.SessionDialerOf(pool)`) dials `pgx.ConnectConfig` with a copy of the pool's
`ConnConfig`, so DSN, credentials, TLS and runtime params (e.g. `search_path` of test schemas) are identical.
A hijacked connection no longer counts toward `MaxConns`; the pool dials a replacement on demand.

Postgres budget per api process: `MaxConns + 2 + held locks (≤ 3)`; worker/preview open no listener.

## 3. ListenerHub

Types (`internal/store/listener_hub.go`, `listen_conn.go`):
- `ListenConn` (base): `Listen(ch)`, `WaitForNotification`, `Ping`, `Close`; `PgListenConn` over `*pgx.Conn`
  (channel quoted with `pgx.Identifier`); tests use a fake.
- `ListenDialer` (base): `Dial(ctx) (ListenConn, error)`; `SessionDialer` in production.
- `NotificationSource` — what consumers depend on: `Subscribe(ch) *Subscription`.
- `Notice{Kind: message|reset, Payload}`; `Subscription.C()` / `Close()`; buffer `SubscriptionBuffer` = 256.
- `ListenerTimings`: backoff 0.5 s → 30 s (×2), keepalive 30 s, ping timeout 5 s.

```
first Subscribe(ch) [any goroutine]
  └─ register subscriber; ch not LISTENed → dirty = true, cancel current wait (wake)
  └─ start.Do → go run() [async]
--
run() [async, one goroutine per process]
  └─ session()
      └─ Dial → error → return (not established)
      └─ forget LISTEN set of the previous session
      └─ syncChannels: LISTEN every subscribed channel → each LISTENed channel: reset notice to its subscribers
      └─ loop waitOnce
          └─ WaitForNotification(ctx with keepalive timeout, cancellable by wake)
              └─ notification → deliver to subscribers of its channel (non-blocking offer)
              └─ hub closing → return
              └─ wait cancelled by wake → syncChannels (new channels LISTENed, their subscribers reset)
              └─ keepalive timeout → syncChannels + Ping → error → session lost
              └─ other error (connection broken) → session lost
  └─ session lost → log warn → sleep backoff (reset to min after an established session) → session()
--
Subscribe(ch) while ch is already LISTENed
  └─ immediate reset notice to the new subscriber (it is live from now on)
--
offer(notice) [under hub lock]
  └─ buffer has room → enqueue
  └─ buffer full → drain the buffer → enqueue one reset (subscriber resyncs; nothing is silently lost)
--
DB.Close
  └─ hub.Close: stop session, close every subscription channel
  └─ health.Close, pool.Close
```

A cancelled `WaitForNotification` leaves the pgx connection usable: pgconn's default deadline context
watcher turns the cancel into a read timeout, and timeouts do not close the connection. Notifications that
arrive during `LISTEN` are buffered by `pgx.Conn` and returned by the next wait.

## 4. Pool sizing

`store.PoolSizing.Apply`: if the DSN contains `pool_max_conns`, pgxpool's own value wins; else
`ROUTER_DB_POOL_MAX_CONNS` (`DatabaseConfig.PoolMaxConns`, > 0); else `max(10, 2 × CPUs)`. The staging compose
file still passes `pool_max_conns=${ROUTER_DB_POOL_MAX_CONNS:-16}` in the DSN; it keeps working and is no longer
required (the same env name can be passed to the process instead).

## 5. Consumers

```
changes.PgListener.Run(ctx) [async]
  └─ Subscribe(router_runtime)
      └─ reset → Broker.Publish(resync hint)            (previously: after every reconnect)
      └─ message → parse {route_id, kind} → Broker.Publish(hint); unparsable → ignored
--
rbac.InvalidationListener.Run(ctx) [async]
  └─ Subscribe(rbac_changed)
      └─ reset → cache.InvalidateAll()                  (previously: on every (re)connect)
      └─ message → Apply(payload): user id → Invalidate(user); unparsable → InvalidateAll
--
PgLeaderElector.Campaign
  └─ pool.Acquire → pg_try_advisory_lock
      └─ lost → Release → ErrNotLeader
      └─ won → Hijack() (conn leaves the pool) → hold: Ping every 2 s
          └─ ctx end / ping fails → pg_advisory_unlock → Close session → lost channel closed
```

`PgListener` loses its `Pool`, `Retry` and `Log` fields (reconnect lives in the hub). Wiring:
`changes.PgListener{Notifications: w.Deps.DB.Notifications(), Broker: broker}` (3 sites in `cmd/api`),
`rbac.NewInvalidationListener(w.Deps.DB.Notifications(), cache, log)`.

## 6. Readiness

`DB.Check` → `HealthProbe.Check(ctx)`: under a mutex, dial the session if absent, `Ping`; a failed ping closes
and forgets the session so the next check redials. The readiness deadline bounds both dial and ping. Readiness
therefore reports "Postgres reachable", independent of pool saturation by request load.

## 7. Tests

- `internal/store/listener_hub_test.go` (fake conn/dialer): per-channel fan-out on one session, new channel
  without reconnect, reconnect after dial failures + resubscribe + reset, keepalive ping failure replaces the
  session, overflow coalesced into one reset, close ends subscriptions, pool sizing defaults.
- `internal/runtime/changes/listener_test.go`: hub notices → broker hints (reset → resync, payload → hint).
- Existing: `rbac` manager tests (`Apply`), opt-in integration `rbac_integration_test.go` (now through
  `DB.Notifications()`), `scheduler/leader_pg_test.go` failover (kills the hijacked session's backend).
