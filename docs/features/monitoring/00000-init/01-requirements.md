# Monitoring — Requirements (00000-init)

## Business intent
Answer "is everything flowing, and if not, where?" within 3 seconds (UI §4). Every number the
UI shows comes from worker metrics (SysDesign §12). Users see platform health on the dashboard,
per-route throughput/lag/latency/errors on the route detail Overview, watch events pass in a
live tail, and get alerted (bell, email, Slack, Teams, webhook) before consumers notice.

## Functional requirements
Phase tags follow UI §13 / SysDesign §14.

### Worker metrics (data side)
- FR-01 [MVP] Workers expose the §12 metric set (`router_events_consumed_total`, `_filtered_total`,
  `_produced_total`, `_dlq_total{error_type}`, `router_consumer_lag{partition}`,
  `router_end_to_end_latency_seconds`, `router_step_duration_seconds{step}`,
  `router_js_timeouts_total`, `router_js_memory_bytes`, `router_runner_state`) labelled
  `route_id`, `branch`, `version`, `worker` through one instrument set the runtime calls.
- FR-02 [MVP] The API queries the metrics store (Prometheus/VictoriaMetrics, PromQL) through a
  configurable backend; a mock backend serves deterministic series for local/FE development.

### Dashboard (UI §4, §12)
- FR-03 [MVP] Health summary: 4 cards — routes running/paused/error (error clickable); events
  forwarded last hour + sparkline; total consumer lag coloured by thresholds; DLQ events last 24 h
  linking to Dead Letters. Thresholds are server config, returned with the summary.
- FR-04 [MVP] Needs attention list: routes in error, routes with growing lag, connections failing
  health checks, routes whose filter matched 0 events (30 min). Each row has one action
  (Open / Retry / Pause / Test) expressed as a serialisable intent.
- FR-05 [P2] Needs attention also lists firing alerts.
- FR-06 [MVP] Recent activity: last 10 route changes (created, edited, deployed, paused,
  resumed, rolled back) with actor and time, read from the audit log.
- FR-07 [MVP] First-run checklist: Add connection → Create route → Deploy, each step done/undone
  derived from data, each linking to its screen; hidden once all are done.
- FR-08 [P3] "Try with demo data" creates an in-memory fake connection (`kind=demo`, connections) with a generated `order-events`
  topic so the builder can be explored with zero setup; idempotent; removable.
- FR-09 [P2] Flow map: graph of connections/topics with routes as animated edges; edge width =
  throughput, red edge = route in error; hover shows route name + events/sec; click opens route;
  filters by connection, team, tag. Built with React Flow.

### Route detail (UI §10.1, §10.2)
- FR-10 [MVP] Overview: pipeline strip with counts per arrow (read → passed filter → sent, plus
  DLQ count) for range 15 min / 1 h / 24 h / 7 d.
- FR-11 [MVP] Overview charts: events/sec in vs out, consumer lag per partition, end-to-end
  latency p50/p95, errors per minute — for the selected range.
- FR-12 [P2] Overview strip shows one row per branch when the route has branches.
- FR-13 [P3] Replay header action (confirm dialog owned here, in the route-detail header): choose a
  timestamp, confirm a duplicate-delivery warning, then call routes' `POST /routes/{id}/replay`
  (offset reset mechanism: route-runtime).
- FR-14 [P3] Live tail over WebSocket: split input/output view per event, outcome badge
  (sent / filtered / dead-lettered / error), pause stream, text filter, only-dropped and
  only-errors toggles. Max 20 events/sec per route. No viewer means zero worker cost.
- FR-15 [P3] Live tail is masked (type-preserving) for Viewers always and for other roles on
  connections marked `sensitive`; every tail open is written to the audit log with its `masked` flag.

### Alerts (UI §11.4, SysDesign §12)
- FR-16 [P2] Per-route alert rules from a simple form: lag above N for M min; error rate above
  X% over M min; throughput 0 for M min; match rate changes by more than X points vs. the
  previous window. Enable/disable, edit, delete.
- FR-17 [P2] Channels per rule: email (SMTP from env config), Slack, Microsoft Teams, webhook. Channel
  secrets (webhook URLs) are stored via the secrets Manager, never returned by the API. The alert
  rule form lives in the route-detail Settings tab.
- FR-18 [P2] Leader-only evaluator every 30 s; deduplicates (one notification per firing, one per
  resolve); keeps alert history (firing → resolved).
- FR-19 [P2] Bell menu in the top bar lists firing/recent alerts with unread count; each opens
  its intent; alerts can be acknowledged.
- FR-20 [P2] `WS /events` pushes route state changes, deployment status, connection health
  and alert fired/resolved so the dashboard, bell and route detail update without reload.

## Non-functional / constraints
- Dashboard summary p95 < 500 ms at 1,000 routes; route metrics p95 < 1 s for 7 d range.
- Metrics store outage degrades gracefully: Postgres-derived counts still render; metric cards
  show "unavailable", no 5xx for the whole dashboard.
- Live tail: Redis flag TTL 60 s renewed while socket open; stream capped; payloads never
  logged; tail cost is zero without a flag.
- Alert notifications at-least-once with dedup key per (rule, firing); never spam on flapping
  (resolve requires one full clean window).
- All endpoints follow platform conventions (RFC 7807 + `path`, cursor pagination, auth/RBAC).
- Code rules: base interface + Manager + config selection for metrics backends, attention
  detectors, alert evaluators, notifiers; enums for kinds, ranges, channels, outcomes.
- Mobile: dashboard, route status, alerts must work at phone width (UI §12).

## Out of scope
- Platform-level monitoring (workers, scheduler, Postgres) — operator's own tools.
- Storing metrics in Postgres; custom PromQL from users; alert routing trees / on-call schedules.
- DLQ browsing/retry (dead-letters), history/diff (routes), audit screen (access-control).

## Dependencies
- route-runtime: calls metric instruments + tail tap in the runner; computes lag; leader
  election; route_runtime change NOTIFY (0008 trigger). routes: replay API used by the dialog.
- platform-core: HTTP/WS server, auth, RFC 7807, config keys, Redis client, migrations runner.
- access-control: RBAC checks, `SampleVisibility` masking decision, audit action enum + reader (activity feed).
- connections: health_status values; `connections.sensitive` (read only); demo connection (`kind=demo`).
- routes: route list/state, pause/resume intents; route detail page shell (header action slot, tab registry) + Settings tab; `RouteMetricsReader` port (implemented here).
- pipeline-engine: event model, branch ids. live-preview: type-preserving masker.
- dead-letters: Dead Letters screen target of DLQ card / Retry intent.
