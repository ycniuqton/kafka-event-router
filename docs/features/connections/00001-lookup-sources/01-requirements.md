# Connections — Requirements (00001-lookup-sources)

Incremental version. **Unchanged:** CN-01..CN-19 and NFR-1..NFR-6 of `00000-init` (Kafka connections) still apply.
This version adds **lookup sources**: connection-like resources that lookup steps (pipeline-engine
`00002-lookups`) call to enrich events.

## Business intent
Enrichment targets (a CRM REST API, a Redis cache, a reference Postgres database, a static price list) are entered
once — address, credentials, limits — and reused by every route, exactly like Kafka connections. Users manage them
in the Connections screen, test them with one click, and never retype secrets inside a route.

## Functional requirements
| ID | Requirement |
| --- | --- |
| LS-01 | Connections screen has a "Lookup sources" tab: name, kind (HTTP / Redis / Postgres / Static), target (base URL, addr, host/db, table), env tag, owner team, status of the last test, used-by route count. Filter by kind/env, text search. |
| LS-02 | Create / edit in a side panel: Basics (name, description, env tag, owner team, sensitive) → Kind settings → Limits. |
| LS-03 | HTTP: base URL (http/https), auth `none` / `bearer` (token) / `basic` (user + password) / `header` (name + value), TLS (skip verify off by default, optional CA PEM), default headers. |
| LS-04 | Redis: addr host:port, db 0..15, optional username + password, TLS. |
| LS-05 | Postgres (read-only): host, port, database, user, password, sslmode, statement timeout (ms, default 2000). Every query runs in a READ ONLY transaction with that statement timeout; only parameterised single SELECT statements are accepted. |
| LS-06 | Static: a library lookup table (`ref` + pinned `version`) — JSON/CSV uploads go through the existing library importer (versioned). |
| LS-07 | Secrets (token, password, header value, client key) are write-only, stored through the `internal/secrets` Manager (Vault Transit in prod); UI shows `••••••` + Replace. |
| LS-08 | Limits per source: max concurrency (default 16), rate limit per second (0 = off) + burst, circuit breaker (failures before open, default 5; open duration ms, default 10 000). |
| LS-09 | Test button for saved sources and unsaved drafts: checklist reachable → authenticated → sample request; plain-language failure reason. Saved tests update status + checked time. |
| LS-10 | RBAC like connections: everyone reads, writes/tests need `connection.write` on the owner team; sensitive flag masks previews like a sensitive connection. |
| LS-11 | Delete is blocked while a route (draft or active version) uses the source (`lookup.sourceId`). |
| LS-12 | Every create / update / delete / secret replace / test is audited (`lookup_source.*`), never with secret values. |
| LS-13 | Workers and preview resolve a source (incl. decrypted secrets) through `GET /internal/v1/lookup-sources/{id}/spec` (workload auth). |

## Non-functional requirements
- LS-NFR-1 Same API conventions as connections (cursor paging, If-Match on PATCH, RFC 7807 with JSON Pointer `path`).
- LS-NFR-2 Source kinds are a registry (`KindSpec` base + `KindManager`); adding a kind adds one implementation.
- LS-NFR-3 A test run is bounded (≤ 10 s per step).
