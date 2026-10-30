# Pipeline Engine — Requirements (00002-lookups)

Incremental version. **Unchanged:** every requirement of `00000-init` (FR-01..FR-60, NFRs) and of `00001-fast-codec`
(built in parallel by another agent) still applies. This version adds lookup / enrichment against external sources.

## Business intent
Routes often need data the event does not carry: the customer tier from a CRM API, a feature flag in Redis, the
region of a store in a Postgres table, a static price list. Today users can only use inline rows or a library
lookup table. Lookup steps let a route call a managed **lookup source** (connections feature, `00001-lookup-sources`)
per event, with caching, timeouts and clear failure semantics, and the Studio previews the real answer.

## Functional requirements
- LK-FR-01 The existing `lookup` step kind gains a third mode: `lookup: {sourceId, request, map, …}` (table modes
  `rows` / `ref+version` are unchanged and keep requiring `field`).
- LK-FR-02 Request templates take event fields with `${<field path>}` placeholders (`${value.customer.id}`). A template
  that is exactly one placeholder keeps the typed value (SQL params, JSON body); others interpolate text.
  Request kinds: `http` (method GET/POST, path, query, headers, JSON body), `redis` (op `get` / `hget` / `mget`,
  key, hashField, keys), `sql` (one SELECT with `:name` params bound from fields — never interpolated), `static` (key).
- LK-FR-03 Response mapping: `map: [{from, to}]`; `from` is a JSONPath-ish pick (`$`, `$.a.b`, `$.items[0].name`),
  `to` a field path. At least one mapping.
- LK-FR-04 `timeoutMs` (default 500, ≤ 10 000) per attempt; `retries` (default 1, ≤ 5) on transient errors only.
- LK-FR-05 `onMiss.action`: `default` (write `onMiss.value` to every target) / `skip` (step is a no-op, default) /
  `drop` (event dropped) / `fail` (typed `lookup_miss` error → route error policy → DLQ).
  A placeholder whose field is missing is a miss (no call is made).
- LK-FR-06 `onError.action` (after retries): `fail` (default) / `skip` / `default`. Transient failures (timeout,
  unavailable, 5xx/429, circuit open, rate limited) fail with the documented engine error type `lookup_unavailable` (shared with dead-letters);
  permanent failures (4xx, bad query, kind mismatch) fail as `step` errors.
- LK-FR-07 Cache per compiled step (= per worker per route version): LRU with `ttlMs`, `negativeTtlMs` (misses),
  `maxEntries`; absent `cache` = no cache.
- LK-FR-08 Per-source concurrency limit, rate limit and circuit breaker (configured on the source, shared by every
  step and route on the worker).
- LK-FR-09 Preview runs lookups against the real source with a short timeout (≤ 2 s) and marks the step
  "live lookup"; a typed mock response per step id (`lookupMocks`) replaces the call ("mock lookup").
- LK-FR-10 Metrics: `router_lookup_requests_total{source,result}`, `router_lookup_duration_seconds{source}`,
  `router_lookup_cache_total{source,result}` (hit ratio = hit / (hit+miss)), `router_lookup_breaker_state{source}`.
- LK-FR-11 Lookup steps declare the fields they read (template placeholders) and write (map targets) for the
  fast-codec field analyser (`00001-fast-codec`).
- LK-FR-12 Batching (MGET / IN within one polled batch) — nice-to-have, deferred (see plan).

## Non-functional requirements
- LK-NFR-1 `pkg/engine` stays Kafka- and driver-free: it owns only the port (`ctxhelpers.SourceGateway`), templates,
  mapping and cache; drivers (net/http, go-redis, pgx) live in `pkg/lookup`.
- LK-NFR-2 Postgres sources run every query in a READ ONLY transaction with `statement_timeout`, parameters bound.
- LK-NFR-3 Secrets of sources never reach logs, traces, metrics labels or the audit diff.
- LK-NFR-4 Coding rules: one base interface + Manager per extensible domain (request kinds, source clients,
  miss/error policies); selection by registry, never by switching on concrete types.
