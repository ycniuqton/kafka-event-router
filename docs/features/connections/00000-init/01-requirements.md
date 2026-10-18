# Connections — Requirements (00000-init)

Sources: UI spec §5 (5.1–5.4), §12, §13; SysDesign §4 (`connections`), §6 (Connections API), §10 (sensitive data), §11, §13, §14.

## Business intent
A **Connection** is a saved Kafka cluster (brokers, security, credentials, optional schema registry),
entered once and reused by every route. Users never retype brokers or passwords inside a route.
The feature must make a broken cluster obvious within seconds (status, plain-language test results)
and let users explore what a cluster holds (topics, partitions, rate, last events) before building routes.

## Functional requirements
| ID | Phase | Requirement |
| --- | --- | --- |
| CN-01 | MVP | List connections: name, env tag (dev/staging/prod, colour-coded), first bootstrap server + "+N", security type, status (healthy / degraded / unreachable / auth failed / not tested) with last-checked time, used-by route count, owner team. Cursor-paginated, filter by env/status, text search. |
| CN-02 | MVP | Create and edit through a 4-step side panel: Basics (name, description, env tag, owner team) → Brokers (chips; pasted comma/space/newline lists split automatically) → Security → Schema registry (optional). |
| CN-03 | MVP | Security modes None (plaintext), TLS-only (ssl), SASL/PLAIN, SASL/SCRAM-256, SASL/SCRAM-512, mTLS; SASL modes may run over TLS. Only the fields of the chosen mode are shown and required. `.pem` drag-and-drop for CA / client certificate / key. |
| CN-04 | P2 | Security mode OAuth (SASL/OAUTHBEARER, client-credentials token endpoint). |
| CN-05 | MVP | Secrets (passwords, private keys, client secrets) are write-only: never returned by any API; UI shows `••••••` + "Replace". Replacing or deleting a connection rotates/deletes the stored secret. |
| CN-06 | MVP | Advanced expander: raw client properties as key/value rows, limited to a documented allow-list; unknown keys are rejected with the offending row highlighted. |
| CN-07 | MVP | Test connection: ordered checklist streamed live — brokers reachable → authenticated → can list topics → can read (sample topic) → can write (optional, chosen test topic). A failed step stops the run, later steps are skipped, and the failure shows a plain-language reason + hint (pattern: what happened, why, what to do). |
| CN-08 | MVP | Test is available for saved connections and for an unsaved draft inside the panel; a saved test updates status + last-checked time. |
| CN-09 | MVP | Background health probe re-checks every connection every 60 s (per-connection opt-out `probeEnabled`) and updates status; failures surface in the dashboard "Needs attention" list (consumed by monitoring). |
| CN-10 | MVP | Topic browser: topics with partitions and message rate (msgs/s, "—" until measurable); internal topics hidden by default; search by name. |
| CN-11 | MVP | Peek: last 20 events of a topic, rendered with the shared event viewer used by the route builder. Served by live-preview `POST /preview/samples` with `count=20` (no own endpoint). |
| CN-12 | MVP | Create topic (name, partitions default 6, replication factor, retention default 7 d) when the connection allows topic creation (opt-in per connection, `allowTopicCreate` default false); also used by the route builder destination step. |
| CN-13 | MVP | Delete is blocked while any route (draft or active version) uses the connection; UI disables the action with a tooltip listing those routes. |
| CN-14 | MVP | Every mutation, test run and peek is written to the audit log; secrets and payloads never appear in logs or audit diffs. |
| CN-15 | MVP | Workers and the preview service resolve a connection (incl. decrypted secret) through an internal, workload-authenticated call, and build Kafka clients through one shared client factory. |
| CN-16 | P2 | Schema registry step: URL + auth (none/basic/bearer, secret write-only); extra test step "schema registry reachable"; decoded Avro/Protobuf/JSON-Schema in Peek. |
| CN-17 | MVP column / P2 toggle | Sensitive flag: `connections.sensitive` column (0004) and its form toggle are owned here; other folders only read it. Samples/peeks/DLQ/tail are masked (type-preserving) for Viewers always and for others on sensitive connections. |
| CN-18 | P2 | Team grants: a connection is owned by one team and may be granted to other teams (grant table + API owned by access-control); routes may only use connections their team owns or is granted. Every user can read every connection; edit/delete/test require editor+ on the owner team. |
| CN-19 | P3 | Demo connection (`kind=demo`): an in-memory fake cluster with an `order-events` topic producing sample events, used by "Try with demo data". |

## Non-functional requirements / constraints
- NFR-1 Secrets: envelope-encrypted via `internal/secrets` Manager; Postgres stores only `secret_ref`; secrets kept in memory only by workers/preview; never logged.
- NFR-2 Test run: each step ≤ 10 s, whole run ≤ 30 s; client disconnect cancels the run; first step result streamed < 1 s after start.
- NFR-3 Topic list ≤ 3 s for 1,000 topics; Peek limits 100 events / 1 MB / 10 s; Peek never joins a consumer group or commits offsets.
- NFR-4 API conventions (owned by platform-core): `/api/v1`, cursor pagination, `If-Match` on PATCH, RFC 7807 with `path` pointing to the failing form field.
- NFR-5 Coding rules (`.devlens/rules.md`): one base interface + Manager per extensible domain (security modes, test checks, failure classifiers, cluster kinds); selection by registry/config, never by switching on concrete types; enums/domain constants only.
- NFR-6 Event payloads never pass through the API process (SysDesign §2): Peek goes through live-preview's sample load, executed in `cmd/preview`.
- NFR-7 Tests: unit (validators, classifiers, splitter), integration via Testcontainers with SASL/SCRAM and mTLS clusters, API contract tests, Playwright create → test → browse flow.

## Out of scope
- Cluster administration beyond create-topic (ACLs, configs editing, partition reassignment, delete topic).
- Consumer-group browsing and offset reset (route replay belongs to routes).
- Secret rotation schedules / external secret sync; Kerberos (GSSAPI) and AWS MSK IAM auth.
- Kafka-protocol proxies; non-Kafka connection types.

## Dependencies on other folders
- platform-core: auth/session, audit-write middleware, `If-Match`/pagination/problem helpers, `internal/secrets` providers, internal workload-auth router, migration numbering, generated web API client.
- access-control: roles + RBAC policy (editor/admin to mutate, open read, raw-data permission), team grants table/API/UI and `CanUseConnection` (P2).
- live-preview: shared event viewer, `POST /preview/samples` (count 20) for Peek, value masker, `pkg/kafka` PartitionSampler (owned there, used by ReadCheck).
- pipeline-engine: route config shape used by usage lookup.
- routes: destination step calls create-topic; route validation calls access-control `CanUseConnection`.
- monitoring: "Needs attention" reads connection health; demo-data trigger (P3).
- route-runtime: workers consume the connection spec + client factory.
