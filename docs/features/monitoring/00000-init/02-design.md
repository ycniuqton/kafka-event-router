# Monitoring — Design (00000-init)

Scope: dashboard, route detail Overview + Live tail, alerts, `WS /events`, worker metric
instruments. Owned code: `internal/metrics`, `internal/alerts`, `internal/livetail`,
`internal/events` (WS hub), `web/src/pages/dashboard`, route-detail `overview` / `tail` tabs,
`web/src/components/charts`, `web/src/app/AlertsBell`.

Cited, not duplicated: RFC 7807 / pagination / auth (platform-core), leader election + runner
loop + lag computation (route-runtime), RBAC + masking + audit writer (access-control), route
CRUD/pause/resume (routes), DLQ screen (dead-letters), event model + branches (pipeline-engine).

---

## 1. Backend packages and key types

### 1.1 `internal/metrics` — instruments (worker side) and queries (API side)

```
internal/metrics/
  instruments/   RouteInstruments      — the §12 metric set, registered on platform/metrics.Registry
                 MetricName            — enum of metric names (no literal strings elsewhere)
                 MetricLabel           — enum: route_id, branch, version, worker, partition, step, error_type
                 RunnerStateValue      — enum gauge values: running=1, paused=2, error=3
  query/         Querier (interface)   — Instant(ctx, Expr) / Range(ctx, Expr, Window) → Series
                 PrometheusQuerier     — HTTP API v1 (/api/v1/query, /query_range); VictoriaMetrics compatible
                 MockQuerier           — deterministic synthetic series seeded by route_id (dev + FE + tests)
                 QuerierManager        — selects implementation by config key ROUTER_METRICS_BACKEND
                 ExprCatalog           — builds PromQL per MetricQuery enum (no ad-hoc strings in services)
                 MetricQuery           — enum: EventsIn, EventsOut, Filtered, DLQ, LagByPartition, LagTotal,
                                         LatencyP50, LatencyP95, ErrorsPerMin, ProducedIncrease, ...
  ranges.go      TimeRange             — enum 15m | 1h | 24h | 7d with Step(): 15s | 30s | 5m | 1h
  thresholds.go  Thresholds            — RouteLagWarn 2000, RouteLagError 20000, TotalLagWarn 10000,
                                         TotalLagError 40000, ZeroMatchWindow 30m (config-overridable)
  dashboard/     SummaryService        — health summary (Postgres counts + metrics)
                 AttentionDetector (interface) — Detect(ctx, Snapshot) → []AttentionItem
                 RouteErrorDetector, LagGrowthDetector, ConnectionHealthDetector,
                 ZeroMatchDetector, FiringAlertDetector (P2)
                 AttentionManager      — runs enabled detectors (config list), sorts by severity
                 ActivityService       — reads audit_log via access-control AuditReader
                 OnboardingService     — derives checklist from counts
                 DemoWorkspaceSeeder (P3) — creates a `kind=demo` connection via connections Service.CreateDemo
                 FlowMapService (P2)   — graph nodes/edges from routes+connections+throughput
  overview/      RouteOverviewService  — pipeline counts + 4 chart series for one route
  http/          DashboardHandler, RouteMetricsHandler, FlowMapHandler, OnboardingHandler (class-based)
```

`RouteInstruments` methods (called by route-runtime, never by other code paths):
`Consumed(labels)`, `Filtered(labels)`, `Produced(labels)`, `DeadLettered(labels, ErrorType)`,
`SetLag(labels, partition, n)`, `ObserveLatency(labels, d)`, `ObserveStep(labels, step, d)`,
`JSTimeout(labels)`, `SetJSMemory(labels, bytes)`, `SetRunnerState(labels, RunnerStateValue)`,
`Forget(labels)` (drop series when a runner stops, avoids stale gauges).

Match rate definition: `matchPct = 1 − filtered / consumed` per window, where
`router_events_filtered_total` counts events **dropped** by the filter (contract with
route-runtime / pipeline-engine).

### 1.2 `internal/alerts`

```
internal/alerts/
  kind.go        AlertRuleKind  — lag_above | error_rate_above | throughput_zero | match_rate_change
                 ChannelType    — email | slack | teams | webhook
                 AlertState     — firing | resolved
  rule.go        AlertRule {ID, RouteID, Name, Kind, Threshold, Window, Channels []ChannelRef, Enabled}
                 ChannelRef {Type, Target (email list) | SecretRef (webhook URL)}
  evaluator/     RuleEvaluator (interface) — Expr(rule) Expr; Breached(sample, rule) bool
                 LagAboveEvaluator, ErrorRateEvaluator, ThroughputZeroEvaluator, MatchRateChangeEvaluator
                 EvaluatorManager — registry AlertRuleKind → RuleEvaluator (registered at boot, no switch)
  notify/        Notifier (interface) — Send(ctx, Notification) error
                 EmailNotifier (SMTP from env config), SlackNotifier, TeamsNotifier, WebhookNotifier (HMAC-signed body)
                 NotifierManager — registry ChannelType → Notifier, enabled set from config
  engine/        AlertEvaluatorJob — leader-only ticker (30 s); StateTracker — dedup / transitions
  store/         RuleRepository, AlertEventRepository (Postgres)
  http/          AlertRulesHandler, AlertsFeedHandler
```

PromQL per kind (built by evaluator, rule values are parameters, `W` = window):
- lag_above: `min_over_time(sum(router_consumer_lag{route_id="R"})[W:30s]) > N`
- error_rate_above: `100 * sum(increase(router_events_dlq_total{route_id="R"}[W])) / sum(increase(router_events_consumed_total{route_id="R"}[W]))`
- throughput_zero: `sum(increase(router_events_produced_total{route_id="R"}[W])) == 0` (only when desired_state = running)
- match_rate_change: match rate over `[W]` vs `[W] offset W`, breached when |Δ| > X points

### 1.3 `internal/livetail` (P3)

```
internal/livetail/
  keys.go        TailKey — flag "router:tail:flag:{route}", stream "router:tail:stream:{route}"
                 TailLimits — RatePerSec 20, FlagTTL 60s, RenewEvery 20s, StreamMaxLen 1000, StreamTTL 120s
                 TailOutcome — sent | filtered | dead_lettered | error
  tap.go         Tap (interface, worker side) — Offer(routeID, TailRecord)
                 RedisTap — cached flag lookup (refresh 2 s), token bucket 20/s, XADD MAXLEN ~
                 NoopTap — used when Redis is not configured
                 TapManager — selected by config
  relay.go       TailSession — per socket: renews flag, XREAD BLOCK, filters, masks, writes frames
                 TailMasker — access-control SampleVisibility (Viewer always; sensitive connection) → live-preview type-preserving masker
  http/          TailHandler (WS /routes/{id}/tail)
```

`TailRecord {at, partition, offset, branch, outcome, step?, error?, input Event, outputs []Event}`.

### 1.4 `internal/events` — `WS /events` hub (P2)

```
internal/events/
  type.go        PushEventType — route.state_changed | deployment.status_changed |
                 connection.health_changed | alert.fired | alert.resolved
  bus.go         Bus (interface) — Publish / Subscribe; RedisBus (pub/sub "router:events"); MemoryBus (tests)
                 BusManager — config-selected
  sources.go     RuntimeChangeSource — LISTEN on route-runtime NOTIFY channel → Bus
  hub.go         Hub — one per API instance; fan-out to sockets filtered by RBAC team visibility
  http/          EventsHandler (WS /events)
```

---

## 2. Data model touchpoints

Reads: `routes`, `route_runtime`, `connections` (health_status), `deployments`, `audit_log`,
`alert_rules`. Writes: `alert_rules`, new `alert_events`.

Migration `00NN_monitoring_alerts` (P2, number ≥ 0010, next free at implementation time):
```sql
CREATE TYPE alert_rule_kind AS ENUM ('lag_above','error_rate_above','throughput_zero','match_rate_change');
ALTER TABLE alert_rules
  ALTER COLUMN kind TYPE alert_rule_kind USING kind::alert_rule_kind,
  ADD COLUMN name text NOT NULL DEFAULT '',
  ADD COLUMN created_by uuid REFERENCES users(id) ON DELETE SET NULL;
CREATE TYPE alert_state AS ENUM ('firing','resolved');
CREATE TABLE alert_events (
  id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  rule_id uuid NOT NULL REFERENCES alert_rules(id) ON DELETE CASCADE,
  route_id uuid NOT NULL REFERENCES routes(id) ON DELETE CASCADE,
  state alert_state NOT NULL, value double precision,
  fired_at timestamptz NOT NULL, resolved_at timestamptz,
  notified_at timestamptz, notify_error text,
  acked_by uuid REFERENCES users(id) ON DELETE SET NULL, acked_at timestamptz);
CREATE UNIQUE INDEX alert_events_one_firing ON alert_events(rule_id) WHERE state = 'firing';
CREATE INDEX alert_events_recent ON alert_events(fired_at DESC);
```
No demo migration: demo data uses connections' in-memory fake connection kind (`kind=demo`, column owned by
connections `0004`).

---

## 3. API endpoints owned (`/api/v1`)

| Method / path | Phase | Request | Response |
| --- | --- | --- | --- |
| GET `/dashboard/summary` | MVP | — | `{routes:{running,paused,error,draft}, eventsLastHour:{total, spark:[n×12]}, totalLag:{value,tone}, dlq24h:{value}, thresholds:{routeLagWarn,routeLagError,totalLagWarn,totalLagError}, metricsAvailable:bool}` |
| GET `/dashboard/attention` | MVP | — | `{items:[{id,kind,tone,title,detail,action:{label,intent}}]}` |
| GET `/activity?limit=10&cursor=` | MVP | — | `{items:[{id,actor,verb,routeId,routeName,detail,at}], nextCursor}` |
| GET `/onboarding` | MVP | — | `{steps:[{id:"connection"|"route"|"deploy", done, href}], allDone, demoAvailable}` |
| POST `/onboarding/demo` | P3 | — | `201 {connectionId, topic}`; `409` if exists |
| DELETE `/onboarding/demo` | P3 | — | `204` |
| GET `/routes/{id}/metrics?range=1h` | MVP | range ∈ 15m,1h,24h,7d | `{range, step, pipeline:[{branch, read, passed, sent, dlq}], series:{eventsIn, eventsOut, lagByPartition:{"0":[..]}, latencyP50, latencyP95, errorsPerMin}}` (series = `[[unixSec,value]]`) |
| GET `/flow-map?connection=&team=&tag=` | P2 | filters | `{nodes:[{id,type:"connection"|"topic",label,connectionId}], edges:[{id,routeId,routeName,from,to,eventsPerSec,status}]}` |
| GET `/alert-rules?routeId=` | P2 | — | `{items:[AlertRule]}` |
| POST `/alert-rules` | P2 | `{routeId,name,kind,threshold,windowMinutes,channels:[{type,target?|secret?}],enabled}` | `201 AlertRule` (secrets write-only) |
| PATCH `/alert-rules/{id}` | P2 | partial + `If-Match` | `200 AlertRule` |
| DELETE `/alert-rules/{id}` | P2 | — | `204` |
| POST `/alert-rules/{id}/test` | P2 | — | `{results:[{channel,ok,error?}]}` sends a test notification |
| GET `/alerts?state=firing&cursor=` | P2 | — | `{items:[{id,ruleId,routeId,routeName,kind,state,value,firedAt,resolvedAt,acked,intent}], unread}` |
| POST `/alerts/{id}/ack` | P2 | — | `204` |
| WS `/events` | P2 | — | frames `{type: PushEventType, at, payload}` |
| WS `/routes/{id}/tail` | P3 | client: `{op:"pause"|"resume"|"filter", outcomes?, text?}` | frames `{type:"record", record: TailRecord, masked}` / `{type:"status", state:"live"|"paused"|"throttled"}` |

RBAC: every read needs `viewer` (read is open across teams); alert rule writes need `editor` on the route's team;
tail masking follows access-control `SampleVisibility`. Mutations are audited. Replay uses routes'
`POST /routes/{id}/replay` (not owned here).

---

## 4. Frontend

```
web/src/api/contracts.ts        extend MetricsService: summary(), routeMetrics(id, range), flowMap(filters)
                                extend AlertsService: list(), attention(), ack(), rules(routeId), saveRule(), deleteRule(), testRule()
                                new ActivityService.list(limit) shape, OnboardingService, LiveTailService.open(), PushEventsService.subscribe()
web/src/api/mock/               MockMetricsService, MockAlertsService, MockLiveTailService (timer-driven),
                                MockPushEventsService, MockOnboardingService
web/src/domain/                 metrics.ts (TimeRange enum, PipelineCounts, Series), alertRule.ts (AlertRuleKind,
                                ChannelType, AlertRuleForm rules), tail.ts (TailOutcome, TailFilter), flowMap.ts
web/src/components/charts/      LineChart, MultiLineChart, Sparkline (SVG, no chart dependency), ChartFrame
web/src/pages/dashboard/        DashboardPage, HealthCards (RoutesCard, ThroughputCard, LagCard, DeadLettersCard),
                                NeedsAttentionList, RecentActivity, FirstRunChecklist, DemoDataCard (P3),
                                FlowMap (P2, React Flow; RouteEdge animated, width from ThroughputScale)
web/src/pages/route-detail/overview/  OverviewTab, PipelineStrip, TimeRangePicker, OverviewCharts
web/src/pages/route-detail/header/    ReplayButton + ReplayDialog (P3, mounted in routes' HeaderActionSlot)
web/src/pages/route-detail/tail/      TailTab, TailControls, TailRow (split input/output), TailSocketClient (reconnect)
web/src/pages/route-detail/settings/  AlertRulesPanel, AlertRuleForm (mounted by routes' Settings tab)
web/src/app/AlertsBell.tsx      bell + dropdown, unread badge, resolves intents (existing ActionIntent)
web/src/store/                  MonitoringActions (refresh, ack), PushEventRouter (applies WS frames to store)
```
Existing FE constants `LagThreshold` and `DashboardStats` are replaced by server `thresholds`
and the summary shape; `Alert`/`AttentionItem` keep their shapes (+ `kind` enum).
Refresh: MVP polls summary/attention every 15 s; P2 switches to `WS /events` push + 60 s poll.

---

## 5. Flows

### 5.1 Dashboard summary
```
HTTP GET /dashboard/summary (user opens dashboard)
└─ auth + RBAC → visible team ids
   └─ SummaryService.Build
      ├─ RouteRepository.CountByState(teams) (Postgres routes ⨝ route_runtime)
      │  └─ observed error wins over desired state → counts {running, paused, error, draft}
      ├─ Querier.Instant(ProducedIncrease 1h) + Range(EventsOut 1h, step 5m) → total + spark
      ├─ Querier.Instant(LagTotal) → value; Thresholds → tone ok|warn|err
      └─ Querier.Instant(DLQ increase 24h) → value
         ├─ metrics store ok → 200, metricsAvailable=true
         └─ metrics store error/timeout (2 s) → 200 with metric fields null, metricsAvailable=false; warn log
```
---
### 5.2 Needs attention
```
HTTP GET /dashboard/attention
└─ AttentionManager.Detect(snapshot of visible routes + connections)
   ├─ RouteErrorDetector: route_runtime.observed_state = error → item(Open → route detail Dead tab)
   ├─ LagGrowthDetector: deriv(lag[15m]) > 0 AND lag > RouteLagWarn → item(Pause intent)
   ├─ ConnectionHealthDetector: health_status ∈ {degraded, unreachable} → item(Test intent), affected route count
   ├─ ZeroMatchDetector: consumed[30m] > 0 AND passed[30m] = 0 → item(Open → builder Filter step)
   └─ FiringAlertDetector (P2): alert_events firing → item(rule intent)
      ├─ one detector fails → its items omitted, others returned; error logged
      └─ merge, dedupe by (route, kind), sort err before warn → 200 {items}
```
---
### 5.3 Route overview metrics
```
HTTP GET /routes/{id}/metrics?range=R
└─ RouteMetricsHandler: parse TimeRange
   ├─ invalid range → 400 problem path="/range"
   ├─ route not visible / missing → 404
   └─ RouteOverviewService.Load(route, R)
      ├─ Instant increase(consumed|filtered|produced|dlq [R]) by branch → pipeline rows
      └─ Range queries (step = R.Step()) in parallel: eventsIn, eventsOut, lagByPartition, p50, p95, errorsPerMin
         ├─ all ok → 200
         └─ metrics store down → 503 problem type "metrics-unavailable" → FE shows retry state
```
---
### 5.4 Alert evaluation
```
[cron] AlertEvaluatorJob tick every 30 s
└─ LeaderGate.IsLeader (route-runtime leader election)
   ├─ not leader → skip tick
   └─ leader → RuleRepository.ListEnabled
      └─ per rule: EvaluatorManager.For(kind).Expr(rule) → Querier.Instant
         ├─ query error → rule skipped this tick; counter router_alert_eval_errors_total++
         └─ StateTracker.Transition(rule, breached)
            ├─ breached AND no firing event → insert alert_events(firing) → [async] notify + Bus.Publish(alert.fired)
            ├─ breached AND already firing → no-op (dedup)
            ├─ not breached AND firing → update resolved_at → [async] notify + Bus.Publish(alert.resolved)
            └─ not breached AND none firing → no-op
```
---
### 5.5 Alert notification delivery
```
[async] NotificationDispatcher.Dispatch(alert_event, rule.channels)
└─ per channel: NotifierManager.For(type)
   ├─ type disabled in config → skip, notify_error="channel disabled"
   └─ SecretResolver.Resolve(secretRef) (internal/secrets) → Notifier.Send
      ├─ ok → alert_events.notified_at = now
      └─ error → notify_error stored → retried on next tick (max 3 attempts, then give up + log)
```
---
### 5.6 Push events (`WS /events`)
```
WS /events (browser opens app shell)
└─ auth → Hub.Register(socket, visible teams)
   └─ [event] Bus message (Redis pub/sub "router:events")
      ├─ source: RuntimeChangeSource (Postgres NOTIFY from route-runtime) → route.state_changed / deployment.status_changed
      ├─ source: connections health check → connection.health_changed
      ├─ source: StateTracker → alert.fired / alert.resolved
      └─ Hub fan-out
         ├─ socket's teams include event team → send frame
         └─ otherwise → drop
            └─ FE PushEventRouter → store update (route badge, bell count, attention refetch)
```
---
### 5.7 Live tail open and stream
```
WS /routes/{id}/tail (user opens Live tail tab)
└─ auth + RBAC viewer; SampleVisibility → masked (Viewer, or sensitive connection) | raw → audit "livetail.opened" {masked}
   ├─ route not running → close 4409 "route not running"
   └─ TailSession.Start
      ├─ SET router:tail:flag:{id} EX 60; renew every 20 s while socket open
      ├─ [async] worker RedisTap sees flag (≤ 2 s cache) → token bucket 20/s → XADD stream MAXLEN ~1000
      │  └─ bucket empty → record dropped, counter tail_dropped++ → status frame "throttled"
      └─ loop XREAD BLOCK 5 s from "$"
         ├─ record matches client filter (outcomes, text) → TailMasker → send frame
         ├─ client paused → discard (no buffering)
         └─ socket closed / ctx done → stop renewing → flag expires ≤ 60 s → workers stop copying
```
---
### 5.8 First-run checklist and demo data
```
HTTP GET /onboarding
└─ counts: connections(kind ≠ demo), routes, deployments(active)
   └─ steps done flags → allDone → FE hides FirstRunChecklist
```
---
```
HTTP POST /onboarding/demo (P3, editor)
└─ DemoWorkspaceSeeder
   ├─ demo connection exists → 409
   └─ connections Service.CreateDemo → connection kind=demo (in-memory fake cluster, seeded `order-events`
      generator owned by connections) → audit "demo.created"
```
---
### 5.9 Replay (P3) — dialog owned here
```
User clicks Replay in the route-detail header → ReplayDialog (timestamp + duplicate warning + type route name)
└─ RoutesService.replay → POST /routes/{id}/replay {from, confirm:true}   (endpoint owned by routes)
   ├─ confirm missing / timestamp in future → 422 path="/from" → dialog shows field error
   └─ 202 {replayId} (route-runtime ReplayCoordinator resets offsets) → dialog closes
      └─ [event] replay progress frames on WS /events → FE toast "Replay started" / "Replay done"
```

---

## 6. Error cases

| Case | Behaviour |
| --- | --- |
| Metrics store unreachable | summary 200 with `metricsAvailable=false`; route metrics 503 `metrics-unavailable`; attention skips metric detectors |
| Unknown `range` / alert kind / channel | 400/422 RFC 7807 with `path` |
| Alert rule for archived route | 422 path=`routeId` |
| Rule `If-Match` mismatch | 412 |
| Notifier failure | stored in `notify_error`, retried 3 ticks, bell still shows the alert |
| Leader lost mid-tick | tick finishes writes in one tx per rule; unique index prevents double firing |
| Redis down | tail WS closes 1011 "live tail unavailable"; Bus falls back to per-instance memory (no cross-instance push) and FE polling continues |
| Viewer without raw permission | tail frames masked; `masked=true` shown as badge |
| High cardinality (`partition`) | lag charts capped at top 50 partitions by lag, rest summed as "other" |

---

## 7. Decisions
1. Replay: the confirm dialog in the route-detail header is owned here. routes owns `POST /routes/{id}/replay`
   and route-runtime owns the offset reset. There is no replay endpoint here.
2. The alert rule form lives in the route-detail Settings tab, owned by routes, which mounts `AlertRulesPanel`.
3. Demo data uses an in-memory fake connection kind (`kind=demo`, connections). There is no
   `connections.origin` column and no bundled-Kafka generator.
4. SMTP settings come from env config (`ROUTER_SMTP_*`).
5. Live tail masking: Viewers are always masked, other roles only on `sensitive` connections. Masks keep types.
6. This folder implements routes' `RouteMetricsReader` for the routes list metrics (`PromRouteMetricsReader`).
7. Route metrics/tail use the route-detail shell (header slot + tab registry) owned by routes.
8. (P2) Migration number is `1090_monitoring_alerts` (block 1090..1099). `alert_events` adds `notify_pending` /
   `notify_attempts` so fired and resolved notifications are retried by the next ticks; per-user acks live in
   `alert_event_acks` (unread = not acked by me, firing or fired in the last 24 h).
9. (P2) Resolve needs one full clean window, tracked in the leader's memory (a leader change restarts the window).
10. (P2) `RuntimeChangeSource` publishes to the local hub only: every API instance LISTENs to `router_runtime`
    itself, so sending these through Redis would duplicate them. Alerts and connection health use the bus.
11. (P2) `WS /api/v1/events` visibility defaults to `open` (reads are open across teams); `ROUTER_EVENTS_VISIBILITY=team`
    restricts frames to the principal's teams.
12. (P2) With several branches, record-level metrics carry only the primary branch (route-runtime), so each branch row
    shows the route's read count and "passed" = sent + DLQ of that branch.
13. (P3) Tail outcomes are `sent | filtered | dropped | dead_lettered | error` (`dropped` = removed by a transform step; a failure that
    was not dead-lettered is `error`). Workers report throttling as a `throttled` stream entry (≤ 1/s), relayed as a status frame.
14. (P3) The worker's flag lookup is cached and refreshed in the background (≤ 2 s), so the runner never blocks on Redis; Redis errors
    read as "not watched". The worker opens its own Redis client for the tap (no readiness dependency on Redis).
15. (P3) The tail's text filter runs on the masked record, so a masked viewer cannot probe raw values through the filter. Close codes
    are sent through the platform `api.WSCloseError` (4409 route not running, 1011 live tail unavailable).
16. (P3) Demo data: `POST/DELETE /onboarding/demo` need `route.write` (editor) and act on the caller's first writable team;
    `GET /onboarding` adds `demo {connectionId, topic}` (null when none). Removing a demo used by routes is a 409 (connections' guard).
17. (P3) Replay lives in the header action slot (`monitoring.replay`, order 5); the confirm needs a past UTC time, the duplicate
    acknowledgement and the typed route name; progress is followed by polling `replayStatus` (toasts started / done / failed).
