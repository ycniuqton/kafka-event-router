# Connections — Design (00000-init)

Requirements: `01-requirements.md` (CN-xx, NFR-x). Conventions owned elsewhere are cited, not repeated:
API envelope/pagination/If-Match/RFC 7807 → platform-core; RBAC → access-control; event viewer + masking → live-preview;
codecs → pipeline-engine.

## 1. Component map

```
web/src/pages/connections  ──HTTP /api/v1──►  cmd/api ──► internal/connections (Service = Manager)
        │                                                    │        │
        │ peek = POST /api/v1/preview/samples (count 20)     │        └─► internal/secrets.Manager
        └──────────► live-preview gateway ──► cmd/preview    │
                                                             └─► pkg/kafka (ClientFactory, Admin)
cmd/worker / cmd/preview ──internal spec call──► internal/connections.SpecResolver ──► pkg/kafka ──► user clusters
```

## 2. Backend — `pkg/kafka` (shared by api, preview, worker)

Existing: `SecurityMode` enum, `ConnectionSpec`. Additions:

| Type | Kind | Responsibility |
| --- | --- | --- |
| `ConnectionSpec` | struct (extend) | add `Kind ClusterKind`, `TLS TLSSpec`, `Auth AuthSpec` (username, mechanism, token URL, scopes), `Properties map[PropertyKey]string`, `Secret SecretBundle` |
| `SecretBundle` | struct | decoded secret fields: `Password`, `ClientKeyPEM`, `ClientSecret`; built from bytes by `SecretCodec` (JSON) |
| `ClusterKind` | enum | `kafka` (MVP), `demo` (P3, in-memory fake connection kind) |
| `SecurityConfigurer` | **base interface** | `Mode() SecurityMode`, `Validate(spec) []FieldError`, `Options(spec) ([]kgo.Opt, error)` |
| `PlaintextConfigurer`, `TLSConfigurer`, `MTLSConfigurer`, `SASLPlainConfigurer`, `SASLScramConfigurer{variant}`, `OAuthBearerConfigurer` (P2) | impls | one per mode; SASL impls compose `TLSOptionBuilder` when `TLS.Enabled` |
| `SecurityRegistry` | **Manager** | `Register(SecurityConfigurer)`, `For(mode)`; populated in `DefaultSecurityRegistry()`; no switch on mode |
| `PropertyKey` + `PropertyCatalog` | enum + Manager | allow-listed advanced properties (`client.id`, `request.timeout.ms`, `metadata.max.age.ms`, `connections.max.idle.ms`, `fetch.max.bytes`, `socket.connection.setup.timeout.ms`) each mapped by a `PropertyMapper` impl to a `kgo.Opt` |
| `ClientPurpose` | enum | `admin`, `sampler`, `producer`, `consumer` — selects purpose-specific defaults |
| `ClientFactory` | **base interface** | `NewClient(ctx, spec, purpose) (Client, error)` |
| `FranzClientFactory` | impl | assembles seeds + security opts + property opts + purpose defaults |
| `DemoClientFactory` (P3) | impl | backed by `kfake` in-process cluster seeded with `order-events` |
| `ClientFactoryRegistry` | **Manager** | selects factory by `spec.Kind` |
| `Admin` | interface | `ListTopics`, `DescribeTopics`, `CreateTopic(TopicSpec)`, `EndOffsets(topics)`, `Ping` (ApiVersions to each seed); P3: `DescribeGroup`, `OffsetsForTimes`, `CommitOffsets` for route-runtime replay |
| `Sampler` | (not owned here) | `PartitionSampler` in `pkg/kafka/sampler.go` is owned by live-preview; `ReadCheck` uses it for 1 record |
| `Record` | struct | partition, offset, timestamp, key, headers, value bytes |
| `FailureReason` | enum | `unreachable`, `dns_failure`, `tls_handshake`, `tls_untrusted`, `auth_failed`, `sasl_mechanism_mismatch`, `not_authorized`, `topic_not_found`, `timeout`, `unknown` |
| `ErrorClassifier` | **base interface** | `Classify(err) (FailureReason, bool)` |
| `NetErrorClassifier`, `TLSErrorClassifier`, `KafkaErrorClassifier` (kerr codes), `SASLErrorClassifier` | impls | ordered chain |
| `FailureClassifier` | **Manager** | runs the chain, falls back to `unknown` |

## 3. Backend — `internal/connections`

| Type | Kind | Responsibility |
| --- | --- | --- |
| `Connection` | entity | mirrors table row + `Security`, `SchemaRegistry`, `Properties`, `Health`, `Version` |
| `EnvTag` | enum | `dev`, `staging`, `prod` |
| `HealthStatus` | enum | `unknown`, `healthy`, `degraded`, `unreachable`, `auth_failed` |
| `Service` | **Manager / entry point** | `List`, `Get`, `Create`, `Update`, `Delete`, `TestSaved`, `TestDraft`, `ListTopics`, `CreateTopic`, `ResolveSpec`, `CanUse` (P2, delegates to access-control `Manager.CanUseConnection`) |
| `Repository` | interface | embeds `store.Repository[Connection, ID]`; `GetByName`, `UpdateHealth`, `ListForProbe` |
| `PgRepository` | impl | `internal/store/connections_repo.go` |
| `ConnectionRule` | **base interface** | `Check(ctx, draft) []FieldError` |
| `NameRule`, `BootstrapRule` (host:port, ≤ 50 seeds), `SecurityRule` (delegates to `SecurityRegistry`), `PropertiesRule` (`PropertyCatalog`), `SchemaRegistryRule` (P2) | impls | — |
| `Validator` | **Manager** | runs configured rules, aggregates → 422 problem with first `path` |
| `SecretVault` | adapter | wraps `secrets.Manager`: `Store(bundle) Ref`, `Replace(ref, patch) Ref`, `Load(ref)`, `Drop(ref)` |
| `UsageResolver` | interface | `RoutesUsing(ctx, connID) ([]RouteRef, error)`, `CountsFor(ctx, ids) map[ID]int` |
| `PgUsageResolver` | impl | jsonb query over `routes.draft_config` + active `route_versions.config` (`source.connectionId`, `branches[*].destination.connectionId`) |
| `TestCheck` | **base interface** | `Step() TestStep`, `Run(ctx, TestSession) StepOutcome` |
| `ReachabilityCheck`, `AuthenticationCheck`, `ListTopicsCheck`, `ReadCheck`, `WriteCheck`, `SchemaRegistryCheck` (P2) | impls | — |
| `TestStep` | enum | `brokers_reachable`, `authenticated`, `list_topics`, `can_read`, `can_write`, `schema_registry` |
| `StepState` | enum | `pending`, `running`, `passed`, `failed`, `skipped` |
| `CheckPlan` | config | ordered `[]TestStep` (`ROUTER_CONNECTION_TEST_STEPS`, default MVP list) |
| `CheckRunner` | **Manager** | executes plan via `CheckRegistry`, emits `StepEvent`s to a `TestSink`, applies per-step timeout, stops on failure |
| `TestSession` | struct | spec, `kafka.Client`, `Admin`, options (`ReadTopic`, `WriteTopic`) |
| `HealthMapper` | domain | first failed step → `HealthStatus` (table in §6) |
| `HintCatalog` | domain | `FailureReason` × `TestStep` → message + hint text |
| `TestSink` | **base interface** | `Emit(StepEvent)`, `Finish(TestResult)` |
| `NDJSONSink` (HTTP), `CollectingSink` (prober) | impls | — |
| `HealthProber` | job | interval loop, runs `CheckRunner` without read/write steps, `UpdateHealth` |
| `TopicBrowser` | domain service | lists topics, hides internal (`__*`, `_schemas`, `router.dlq.*` shown), attaches rate |
| `RateEstimator` | interface | `Rates(ctx, connID, endOffsets) map[topic]*float64` |
| `RedisRateEstimator` | impl | stores last end-offset snapshot per connection (TTL 10 min); rate = Δsum / Δt; ≥ 5 s apart else previous rate |
| `TopicCreator` | domain service | validates `TopicSpec` (legal chars `[a-zA-Z0-9._-]`, ≤ 249, partitions 1..1000), checks `AllowTopicCreate`, calls `Admin.CreateTopic` |
| `TopicDefaults` | domain constants | partitions 6, retention 7 d, replication = broker default (`-1`) |
| `SpecResolver` | domain service | row + `SecretVault.Load` → `kafka.ConnectionSpec` |
| HTTP handlers | class-based (`http.Handler` structs) | `ListHandler`, `GetHandler`, `CreateHandler`, `UpdateHandler`, `DeleteHandler`, `TestHandler`, `TestDraftHandler`, `TopicsHandler`, `CreateTopicHandler`, `SpecHandler`; mounted by `Router.Mount(chi.Router)` |
| `ProblemType` additions | constants | `connection-in-use` (409), `topic-exists` (409), `topic-create-disabled` (403), `cluster-unavailable` (502, carries `reason`) |

## 4. Data model touchpoints

Existing table `connections` (0001) is reused. Migration `0004_connections` (fixed MVP number). This folder
owns every `connections.*` column; other folders only read them:

```sql
ALTER TYPE connection_health ADD VALUE IF NOT EXISTS 'auth_failed';
ALTER TABLE connections
  ADD COLUMN description        text    NOT NULL DEFAULT '',
  ADD COLUMN sensitive          boolean NOT NULL DEFAULT false,
  ADD COLUMN allow_topic_create boolean NOT NULL DEFAULT false,   -- opt-in per connection
  ADD COLUMN probe_enabled      boolean NOT NULL DEFAULT true,    -- per-connection health-probe opt-out
  ADD COLUMN kind               text    NOT NULL DEFAULT 'kafka' CHECK (kind IN ('kafka','demo')),
  ADD COLUMN version            bigint  NOT NULL DEFAULT 1,
  ADD COLUMN last_check         jsonb,          -- {step, reason, message, hint} of last failure
  ADD CONSTRAINT connections_env_tag_chk CHECK (env_tag IN ('dev','staging','prod'));
```

- `config` JSONB holds non-secret security + properties: `{tls:{enabled,caPem,serverName}, auth:{username,mechanism,tokenUrl,scopes}, properties:{...}}`.
- `secret_ref` → one envelope-encrypted `SecretBundle` (JSON) per connection.
- `schema_registry` JSONB (P2): `{url, auth:{mode, username}, secretRef}`.
- Grants (P2): table `team_connection_grants` and its API are owned by access-control; connections only reads it through `CanUseConnection`.
- Redis: `conn:rate:<id>` (end-offset snapshot, TTL 10 min).
- Audit rows (via platform-core middleware): `connection.create|update|delete|test|topic.create`; peeks are audited by live-preview as `samples.view`; diffs exclude secrets.

## 5. API (owned endpoints, all under `/api/v1`)

### Resource shape
```json
{
  "id": "6f1c…", "name": "orders-prod", "description": "", "envTag": "prod",
  "owner": { "teamId": "…", "teamName": "Platform" },
  "bootstrapServers": ["kafka-1.orders.internal:9093", "kafka-2.orders.internal:9093"],
  "security": {
    "mode": "sasl_scram_512",
    "tls": { "enabled": true, "caPem": "-----BEGIN…", "serverName": null },
    "username": "router-svc",
    "secretFields": { "password": "set" }
  },
  "schemaRegistry": null,
  "properties": { "client.id": "event-router" },
  "sensitive": false, "allowTopicCreate": false, "probeEnabled": true, "kind": "kafka",
  "health": { "status": "auth_failed", "checkedAt": "2026-10-13T08:00:00Z",
              "lastFailure": { "step": "authenticated", "reason": "sasl_mechanism_mismatch",
                               "message": "Authentication failed.", "hint": "Check the SASL mechanism: the broker expects SCRAM-SHA-512." } },
  "usedBy": { "count": 2, "routes": [{ "id": "…", "name": "order-paid-to-partner" }] },
  "version": 4, "createdAt": "…", "updatedAt": "…"
}
```
`secretFields` values are only `"set"` / `"missing"`. `usedBy.routes` is returned on GET-one; list returns `count` only.

### Endpoints
| Method / path | Phase | Request | Response / errors |
| --- | --- | --- | --- |
| `GET /connections?cursor&limit&env&status&q` | MVP | — | `{items:[Connection], nextCursor}` |
| `POST /connections` | MVP | resource fields minus server fields + `"secrets": {"password": "…", "clientKeyPem": "…", "clientSecret": "…"}` | 201 + resource, `ETag: "<version>"`; 422 `path` e.g. `/security/username`; 409 `/name` |
| `GET /connections/{id}` | MVP | — | resource + `ETag` |
| `PATCH /connections/{id}` | MVP | JSON merge patch; `If-Match` required; `secrets` keys present = replace, absent = keep | 200; 412 stale; 422 |
| `DELETE /connections/{id}` | MVP | — | 204; 409 `connection-in-use` with `routes:[{id,name}]` |
| `POST /connections/{id}/test` | MVP | `{readTopic?: string, writeTopic?: string}` | 200 `application/x-ndjson` stream (below) |
| `POST /connections/test` | MVP | full draft (as POST body) + `{readTopic?, writeTopic?}`; `connectionId?` to reuse stored secrets when editing | same stream; no health update |
| `GET /connections/{id}/topics?q&includeInternal` | MVP | — | `{items:[{name, partitions, replicationFactor, internal, ratePerSec: number|null}]}`; 502 `cluster-unavailable` |
| `POST /connections/{id}/topics` | MVP | `{name, partitions?, replicationFactor?, retentionMs?, configs?}` | 201 `{name, partitions, replicationFactor, retentionMs}`; 403 `topic-create-disabled`; 409 `topic-exists`; 422 `/name` |
| `GET /internal/connections/{id}/spec` | MVP | workload identity | `kafka.ConnectionSpec` JSON incl. secret; never exposed on public router |

Peek has no endpoint here: the panel calls live-preview `POST /api/v1/preview/samples` with
`{connectionId, topic, mode: "latest", count: 20}` (decision). Grants use access-control's
`/teams/{id}/grants` (P2).

### Test stream (NDJSON, one JSON object per line, flushed per event)
```
{"type":"step","step":"brokers_reachable","state":"running"}
{"type":"step","step":"brokers_reachable","state":"passed","durationMs":84}
{"type":"step","step":"authenticated","state":"failed","reason":"sasl_mechanism_mismatch","message":"Authentication failed.","hint":"Check the SASL mechanism: the broker expects SCRAM-SHA-512."}
{"type":"step","step":"list_topics","state":"skipped"}
{"type":"result","ok":false,"health":"auth_failed","checkedAt":"2026-10-13T08:00:00Z"}
```

## 6. Health mapping (`HealthMapper`)
| First failed step | Health |
| --- | --- |
| none (write skipped or passed) | `healthy` |
| `brokers_reachable` | `unreachable` |
| `authenticated` | `auth_failed` |
| `list_topics`, `can_read`, `can_write`, `schema_registry` | `degraded` |

## 7. Flows

```
[user action] Save new connection (panel step 4 → Save)
└─ POST /connections  (auth + RBAC editor on owner team — access-control)
   └─ CreateHandler decodes body → Service.Create
      └─ Validator runs NameRule, BootstrapRule, SecurityRule, PropertiesRule
         ├─ any FieldError → 422 problem, path = first error field
         └─ ok → SecretVault.Store(secrets) → secret_ref
            └─ TxManager: Repository.Create
               ├─ unique (team_id,name) violated → SecretVault.Drop(ref); 409 path "/name"
               └─ ok → 201 + ETag; audit "connection.create" (secrets redacted)
                  └─ [async] UI immediately calls POST /connections/{id}/test
```
---
```
[user action] Edit connection (PATCH with If-Match)
└─ PATCH /connections/{id}
   └─ UpdateHandler → Service.Update(id, patch, ifMatch)
      └─ Repository.Get
         ├─ version ≠ If-Match → 412 problem
         └─ merge patch → Validator
            ├─ invalid → 422 path
            └─ valid
               ├─ patch.secrets present → SecretVault.Replace(ref, patch) → new ref; old ref dropped after commit
               └─ Repository.Update (version+1, health → unknown when brokers/security changed) → 200; audit "connection.update"
```
---
```
[user action] Delete connection
└─ DELETE /connections/{id}
   └─ DeleteHandler → Service.Delete
      └─ UsageResolver.RoutesUsing(id)
         ├─ ≥ 1 route → 409 connection-in-use + routes[] (UI tooltip lists them)
         └─ none → TxManager: Repository.Delete → after commit SecretVault.Drop(ref) → 204; audit "connection.delete"
```
---
```
[user action] Test saved connection
└─ POST /connections/{id}/test {readTopic?, writeTopic?}
   └─ TestHandler → Service.TestSaved → SpecResolver (row + SecretVault.Load)
      └─ ClientFactoryRegistry.For(kind).NewClient(spec, admin) → TestSession
         └─ CheckRunner.Run(plan, session, NDJSONSink)   (ctx cancelled on client disconnect)
            └─ for each step: emit running → TestCheck.Run (timeout 10 s)
               ├─ passed → emit passed + durationMs → next step
               ├─ can_write with no writeTopic → emit skipped → next step
               └─ failed → FailureClassifier.Classify(err) → HintCatalog → emit failed; remaining steps → skipped
            └─ HealthMapper → Repository.UpdateHealth(status, checkedAt, lastFailure)
               └─ emit result line; audit "connection.test"; health change → [event] monitoring needs-attention refresh
```
---
```
[user action] Test unsaved draft (panel "Test" before Save)
└─ POST /connections/test {draft, connectionId?}
   └─ TestDraftHandler → Validator
      ├─ invalid → 422 path (no stream opened)
      └─ valid → connectionId given and draft secret missing → merge stored SecretBundle
         └─ same CheckRunner path as above → result line; no health update; audit "connection.test"
```
---
```
[cron] HealthProber tick (every ROUTER_CONNECTION_PROBE_INTERVAL, default 60 s, leader-only)
└─ Repository.ListForProbe (kind = kafka, probe_enabled = true, last_checked_at older than interval)
   └─ per connection [async, bounded worker pool = 8]
      └─ CheckRunner.Run(probe plan: reachable, authenticated, list_topics; CollectingSink)
         ├─ health unchanged → UpdateHealth(checkedAt only)
         └─ health changed → UpdateHealth(status, lastFailure) → [event] connection.health_changed (WS /events via monitoring)
```
---
```
[user action] Open topic browser
└─ GET /connections/{id}/topics?q
   └─ TopicsHandler → Service.ListTopics → SpecResolver → Admin
      ├─ cluster error → FailureClassifier → 502 cluster-unavailable {reason, hint}
      └─ ok → Admin.ListTopics + Admin.EndOffsets
         └─ RedisRateEstimator.Rates(previous snapshot)
            ├─ no / too-recent snapshot → ratePerSec null ("—"); store snapshot
            └─ snapshot ≥ 5 s old → rate = Δoffsets / Δt; store snapshot
               └─ TopicBrowser hides internal topics unless includeInternal → 200 items
```
---
```
[user action] Create topic (topic browser or route builder destination step)
└─ POST /connections/{id}/topics {name, partitions?, retentionMs?}
   └─ CreateTopicHandler → Service.CreateTopic → TopicCreator
      ├─ connection.allowTopicCreate = false → 403 topic-create-disabled
      ├─ invalid name / partitions → 422 path
      └─ ok → apply TopicDefaults → Admin.CreateTopic
         ├─ TOPIC_ALREADY_EXISTS → 409 topic-exists
         ├─ not authorized → 502 cluster-unavailable reason not_authorized
         └─ created → 201; audit "connection.topic.create"
```
---
```
[user action] Peek (last 20 events) in TopicBrowserDrawer
└─ PeekPanel → PreviewService.loadSamples({connectionId, topic, mode: latest, count: 20})
   └─ POST /api/v1/preview/samples   (owned by live-preview; executed in cmd/preview)
      ├─ topic missing → 404 problem → panel shows "Topic not found"
      └─ ok → session sample page → shared event viewer
         └─ masking (Viewer always, or sensitive connection) applied by live-preview
```
---
```
[event] Worker/preview runner starts and needs a cluster client
└─ GET /internal/connections/{id}/spec   (workload identity — platform-core)
   └─ SpecHandler → SpecResolver → SecretVault.Load(ref)
      ├─ secret missing / decrypt error → 500, worker marks runner error (route-runtime)
      └─ ok → spec JSON (memory only) → ClientFactoryRegistry → kafka client; audit "secret.read" by service
```

## 8. Frontend — `web/src/pages/connections`

Existing pieces reused: `ConnectionsService` (contracts), `ConnectionActions`, `ConnectionTestSimulator` (mock), `components/ui/*` (Drawer, SegmentedControl, Chip, EnvTag, HealthDot, Modal).

| File / component | Responsibility |
| --- | --- |
| `domain/connection.ts` (extend) | add `name`, `description`, `kind`, `sensitive`, `allowTopicCreate`, `usedBy`, `version`, `health.lastFailure`; `ConnectionStatus.Degraded`; `SecurityMode` gains `Tls`, `SaslScram256`, `SaslScram512`; `TestStep` enum aligned with BE ids; `TopicInfo.ratePerSec: number | null` with formatter |
| `api/http/HttpConnectionsService.ts` | REST implementation registered as `DataSource.Http`; maps BE health → `ConnectionStatus` via `HealthStatusMap` |
| `api/http/NdjsonStreamReader.ts` | reads `fetch` body lines → `StepEvent`s |
| `api/http/ConnectionTestAssembler.ts` | folds `StepEvent`s into `ConnectionTestRun` snapshots for `onUpdate` |
| `pages/connections/ConnectionsPage.tsx` | table (CN-01), filters, empty state "No connections yet…" + "+ New connection" |
| `pages/connections/ConnectionRowActions.tsx` | Edit / Test / Browse topics / Delete (disabled + tooltip of `usedBy.routes`) |
| `pages/connections/panel/ConnectionPanel.tsx` | 4-step side panel (Drawer), step state machine `ConnectionPanelFlow` |
| `panel/BasicsStep.tsx`, `BrokersStep.tsx`, `SecurityStep.tsx`, `SchemaRegistryStep.tsx`, `AdvancedProperties.tsx` | step bodies |
| `panel/BootstrapListParser.ts` | splits pasted text on `,` / whitespace / newline, dedupes, validates host:port |
| `panel/security/SecurityFormRegistry.ts` | maps `SecurityMode` → field component (`NoneFields`, `TlsFields`, `SaslFields`, `MtlsFields`, `OAuthFields` P2) — no switch |
| `panel/security/PemDropZone.tsx` | drag-and-drop `.pem` → text |
| `panel/security/SecretField.tsx` | write-only input: `••••••` + "Replace" when `secretFields[x] = set` |
| `pages/connections/test/TestChecklist.tsx` | live ticks from `connectionTests[id]`; reason + hint on failure; optional read/write topic pickers |
| `pages/connections/topics/TopicBrowserDrawer.tsx` | topic table, search, "show internal" toggle, Peek, Create topic |
| `pages/connections/topics/CreateTopicDialog.tsx` | name/partitions/retention with defaults 6 / 7 d |
| `pages/connections/topics/PeekPanel.tsx` | `PreviewService.loadSamples` (count 20) + shared event viewer (live-preview) |
| `ProblemFieldMapper` (platform-core generic) | RFC 7807 `path` → panel step + field highlight |

```
[user action] Paste "a:9092, b:9092\nc:9092" into Brokers step
└─ BootstrapListParser.split → ["a:9092","b:9092","c:9092"]
   ├─ invalid token → chip shown red with "host:port expected"
   └─ valid → chips added; duplicates ignored
```
---
```
[user action] Click Test in panel or row
└─ ConnectionActions.test(id | draft)
   └─ services.connections.test → HttpConnectionsService
      └─ fetch POST …/test → NdjsonStreamReader [async stream]
         └─ each line → ConnectionTestAssembler → onUpdate(run) → store.connectionTests[id] → TestChecklist re-renders
            ├─ result ok → toast "Connection healthy"; list refresh
            └─ result failed → failed row shows message + hint; status badge updated
```

## 9. Error cases
| Case | Behaviour |
| --- | --- |
| Validation (missing username for SASL, bad host:port, unknown property) | 422, `path` = JSON Pointer of the field (`/bootstrapServers/2`, `/security/username`, `/properties/foo`) |
| Duplicate name in team | 409 `path: /name` |
| Stale edit | 412 → UI reloads and shows "changed by someone else" |
| Delete while used | 409 `connection-in-use` with routes |
| Cluster unreachable / auth fails during topics/create | 502 `cluster-unavailable` + `reason` + hint text from `HintCatalog` |
| Test step timeout | step `failed`, reason `timeout`; overall cap 30 s |
| Client closes test stream | context cancelled, clients closed, no health update |
| Secret decrypt failure | 500 problem (no detail leaked); logged with connection id only |
| Write check without permission | `can_write` failed `not_authorized` → health `degraded` |

## 10. Decisions
- Peek reuses live-preview `POST /preview/samples` with `count=20`. There is no peek endpoint, no `Peeker` and no
  sampler here; `pkg/kafka` PartitionSampler is owned by live-preview.
- OAuth (SASL/OAUTHBEARER) is P2.
- Topic creation is opt-in per connection (`allow_topic_create` default false).
- The health check runs every 60 s; each connection can opt out (`probe_enabled`, toggle in the Basics step).
- `connections.sensitive` (column, API field, form toggle) is owned here; other folders only read it.
  Masking keeps types (strings → masked string, numbers → 0) and is applied by live-preview/dead-letters/monitoring.
- Users can read other teams' connections. Writes need editor+ on the owner team.
- Team grants (P2) use access-control's `team_connection_grants` table and API. `Service.CanUse` delegates there.
- Demo data uses an in-memory fake connection kind (`kind=demo`, P3).
- Migration number: `0004_connections`.
- Problem `path` values are JSON Pointers.
- P2 (as built): the registry password/token is the `schemaRegistrySecret` field of the one `SecretBundle`; the registry
  spec lives in the existing `connections.schema_registry` jsonb (no migration). `pkg/schemaregistry` owns registry auth
  modes (`Authenticator` + `AuthRegistry`) and the `GET /subjects` probe. On update the bundle keeps only the secrets the
  security mode and registry auth still use.
- P2 (as built): optional checklist steps come from `PlanExtensions` in `CheckRunner`; `schema_registry` is appended to
  the test and probe plans when a registry is configured (FE adds its row when the stream reports it).
- P2 (as built): item routes resolve the owner team (`Service.TeamOf`) for the RBAC engine; create and draft test check
  write on the body / stored connection's team (`TeamGuard`). Granted teams in the panel come through the FE port
  `ConnectionGrantsSource` that access-control implements.

- P3 (as built): demo data. `kind=demo` connections are served by `kafka.DemoClientFactory`; every process
  (api, preview, worker) hosts its own in-memory kfake cluster per demo connection, fed by the same
  deterministic generator. Brokers/security of a demo row are placeholders; the kind is immutable.

```
Service.CreateDemo(team)                       (called by monitoring POST /onboarding/demo)
└─ DemoOf(team) found → DemoExistsError (409)
└─ not found → insert connections row kind=demo, probe off, topic create on → audit connection.created
---
any client for a kind=demo spec (test, topics, preview peek, worker runner)
└─ ClientFactoryRegistry / OpenerManager → DemoClientFactory
   └─ DemoClusterPool.Acquire(connection id)
      ├─ running → seeds of that cluster
      └─ absent → start kfake, create order-events (3 p, 6 h retention), produce 120-event backlog
         └─ [async] feed: 1 order event / s → order-events
---
Service.Delete / DeleteDemo
└─ routes use it → InUseError (409)
└─ unused → delete row + secret → audit → ClusterTeardown.Release → api-side demo cluster stopped
```
- P3 (as built): replay support in `Admin`: `DescribeGroup`, `OffsetsForTimes`, `CommitOffsets` (route-runtime#RR-203).

## 11. Open questions (remaining)
1. Should "can write" create the test topic automatically if missing, or only use an existing one (this design: existing only)?
