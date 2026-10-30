# Pipeline Engine — Design (00002-lookups)

Incremental. **Unchanged:** sections 1–12 of `00000-init/02-design.md` and everything in `00001-fast-codec`.
Only the lookup-source mode of the `lookup` step, its port, runtime library and error contract are new.

## 1. Route document (schema `route.v1`, additive)

The `lookup` step gets one new member `lookup` (`$defs/lookupSpec`, title `LookupSpec`). With `lookup` present the
step is a source lookup; `field` is then not required (the `if lookup then` rule is `anyOf [field+rows,
field+ref+version, lookup]`).

```json
{ "kind": "lookup", "id": "crm",
  "lookup": {
    "sourceId": "6f0c…",                                   // lookup source id (connections 00001)
    "request": { "kind": "http", "method": "GET", "path": "/customers/${value.customerId}",
                 "query": { "fields": "tier" }, "headers": { "x-tenant": "${headers.tenant}" }, "body": null },
    "map": [ { "from": "$.tier", "to": "value.customer.tier" }, { "from": "$", "to": "value.customer.raw" } ],
    "timeoutMs": 500, "retries": 1,
    "onMiss":  { "action": "default", "value": "standard" },   // default | skip (default) | drop | fail
    "onError": { "action": "fail" },                            // fail (default) | skip | default (+ value)
    "cache":   { "ttlMs": 60000, "negativeTtlMs": 10000, "maxEntries": 10000 } } }
```

| request.kind | members | answer value (what `$` is) | miss |
| --- | --- | --- | --- |
| `http`   | `method` GET/POST, `path`, `query{}`, `headers{}`, `body` (any; strings are templates) | JSON body (text when not JSON) | 404 |
| `redis`  | `op` get / hget / mget, `key`, `hashField` (hget), `keys[]` (mget) | value (JSON-decoded when it parses) / array for mget | nil (all nil for mget) |
| `sql`    | `sql` (one SELECT, `:name` params), `params{name: template}` | first row as object | zero rows |
| `static` | `key` | row value of the library table | key absent |

Templates: `${path}` placeholders (engine field paths). Exactly one placeholder → typed value; otherwise text.
HTTP path placeholders are path-escaped, query values url-encoded. Missing placeholder field → miss, no call.
SQL `:name` params are rewritten to `$1..$n` at compile; values are always bound. Request kind must match the
source kind (`http`↔http, `redis`↔redis, `sql`↔postgres, `static`↔static); a mismatch is a permanent error.

## 2. Packages

```
pkg/engine/ctxhelpers/source.go      PORT: SourceGateway {Fetch, Observe}, SourceCall, SourceRequest, SourceAnswer,
                                     SourceError{Transient} (Temporary()), ErrSourceUnavailable, NoSourceGateway
pkg/engine/enrich/                    step side (driver-free)
  template.go      Template (compiled `${}` text), TemplateSet (map/list of templates), FieldRefs
  request.go       RequestCompiler base + RequestCompilerManager (http, redis, sql, static) → RequestBuilder
  sqlparams.go     SQLParams: `:name` → `$n` rewrite + single-statement guard
  mapping.go       ResponsePick (`$.a.b[0]` via path.Parser on a synthetic event), Mapping list
  policy.go        MissPolicy / ErrorPolicy base + PolicyManager (default, skip, drop, fail)
  cache.go         AnswerCache: LRU + TTL + negative TTL + max entries (per compiled step)
  step.go          SourceLookup step: render → cache → gateway (timeout, retries) → map / policy
  compiler.go      Compiler used by recipes.Lookup when `lookup` is set; FieldAccess (reads/writes)
pkg/lookup/                           runtime side (drivers): implements ctxhelpers.SourceGateway
  kinds.go spec.go          SourceKind, SourceSpec (+SecretBundle, Limits) — JSON shared with the API spec endpoint
  client.go                 Client base {Fetch, Ping, Close}, ClientFactory base, FactoryRegistry (Manager)
  http.go redis.go postgres.go static.go   one client per kind
  breaker.go limiter.go guard.go           CircuitBreaker, Limiter (semaphore + x/time/rate), GuardedClient
  gateway.go                Gateway: SpecProvider → guarded clients (per source id, spec TTL refresh) + Observer
  metrics.go                Observer base, NoopObserver, PromObserver (router_lookup_*)
  provider_remote.go        RemoteSpecProvider over controlplane GetJSON `/internal/v1/lookup-sources/{id}/spec`
  preview.go                PreviewGateway: per-step mocks first, else live with a capped timeout (mode live/mock)
  tester.go                 Tester: reachable → authenticated → sample (API Test button)
```

`ctxhelpers.Set` gets one new member `Sources SourceGateway` (default `NoSourceGateway`: every call fails with the
permanent `ErrSourceUnavailable`). It already travels through `CompileOptions.Helpers` → `StepContext.Helpers`,
so the compile pipeline itself is unchanged. Callers set it: worker (`reconcile.EngineVersionCompiler`),
preview (`PipelineBuilder`), API (DLQ dry-run/retry `PipelineCache.WithSources`, saved test cases `RunnerDeps.Sources`,
both fed the in-process `lookupGateway()`).

## 3. Per-event flow

```
[event] StepChain.runStep → SourceLookup.Apply(ev)
└─ RequestBuilder.Render(ev)
   ├─ placeholder field missing → MissPolicy.Apply
   └─ ok → cache key = request kind + canonical request
      └─ AnswerCache.Get(key)
         ├─ hit found        → Mapping.Write(answer) → Observe(cache hit)
         ├─ hit negative     → MissPolicy.Apply      → Observe(cache hit)
         └─ miss             → Observe(cache miss)
            └─ attempt loop (1 + retries), each with context.WithTimeout(timeoutMs)
               └─ Gateway.Fetch(call)
                  └─ GuardedClient: breaker.Allow → limiter.Acquire → Client.Fetch → breaker.Record
                     ├─ breaker open      → SourceError{Transient}                → metrics result=circuit_open
                     ├─ rate/concurrency  → wait until ctx deadline, then Transient → result=rate_limited
                     ├─ found             → cache.Put(found)  → Mapping.Write      → result=hit
                     ├─ not found         → cache.PutMiss     → MissPolicy.Apply   → result=miss
                     ├─ transient error   → retry; exhausted → ErrorPolicy.Apply   → result=error
                     └─ permanent error   → ErrorPolicy.Apply (no retry)          → result=error
```

Miss policies: `skip` → event unchanged; `default` → `value` written to every `map[].to`; `drop` → zero events;
`fail` → `LookupFailure{Type: lookup_miss}`. Error policies: `fail` → `LookupFailure{Type: lookup_unavailable, Cause: DependencyError{Kind: lookup}}`
for transient causes, else a plain step error; `skip`; `default`.

```
[preview run] PreviewRunner.Run → PipelineBuilder.options → Helpers.Sources = PreviewGateway{mocks, live, cap 2 s}
└─ SourceLookup.Apply → PreviewGateway.Fetch(call{StepID})
   ├─ mocks[stepId] present → answer mode=mock (no I/O)
   └─ else live Gateway (timeout min(step, 2 s)) → answer mode=live
└─ step log line `lookup: mode=<live|mock|cache> source=<id> result=<hit|miss|error> ms=<n>` (Studio badge)
```

## 4. Error contract (shared with dead-letters 00001-workflow §2.1)

One name end to end: a transient lookup failure is `lookup_unavailable` as engine `ErrorType`, as DLQ
`router.error_type` and as API retry/dry-run failure kind. The step wraps the cause in
`engine.DependencyError{Kind: lookup, Name: <sourceId>}` so the runtime classifier, the DLQ type rules and the
retry-topic policy all recognise it from the same contract (`LookupFailure.Error()` hides the wrapper's text).

| engine `ErrorType` | when | runtime category → disposition | DLQ `router.error_type` (class) |
| --- | --- | --- | --- |
| `lookup_unavailable` (`engine.ErrLookupUnavailable`) | timeout, unavailable, 5xx/429, circuit open, rate limited, after retries, `onError=fail` | `dependency_transient` → `apply_policy`; eligible for `errorPolicy.retry` tier topics | `lookup_unavailable` (transient) |
| `lookup_miss` (`engine.ErrLookupMiss`) | `onMiss=fail` | `step` → `apply_policy` (data error) | `step_error` (data) |
| `step` | permanent source errors (4xx, bad SQL, kind mismatch, no gateway) with `onError=fail` | `step` → `apply_policy` | `step_error` |

Mechanism: `step.TypedError` (optional interface `EngineErrorType() engine.ErrorType`) — `ProcessErrors.ForStep`
asks it before the timeout/limit checks, so other step kinds can declare types the same way. `SourceError`
implements `Temporary()` so `failures.TransientCauses` also recognises it.

## 5. Field access (for the fast-codec analyser, 00001-fast-codec)

`enrich.SourceLookup` implements `step.FieldAccessor` (`FieldAccess() step.FieldAccess{Reads, Writes, FullTree}`):
reads = every template placeholder path, writes = every `map[].to`; `FullTree` false. The interface is declared
in `pkg/engine/step/access.go` (new, small). If 00001-fast-codec lands its own interface, the orchestrator keeps one
and adapts this method (merge point flagged).
Integration: 00001-fast-codec analyses routes through its compile-time `projection.Recorder` (paths parsed via
`Scope.Path`). `enrich.RequestAccess` feeds `FieldAccess().Reads` into that recorder at compile; `map[].to` already
goes through `Scope.Path`. So `lookup` (table and source mode) stays in `projection.PathBoundedSteps` and lookup
routes run the fast codec (PE-FC-64).

## 6. Metrics

| metric | labels | meaning |
| --- | --- | --- |
| `router_lookup_requests_total` | `source`, `result` = hit / miss / error / timeout / circuit_open / rate_limited | calls that reached the gateway |
| `router_lookup_duration_seconds` | `source` | histogram of gateway latency (buckets 1 ms … 5 s) |
| `router_lookup_cache_total` | `source`, `result` = hit / miss | cache hit ratio = `hit / (hit + miss)` |
| `router_lookup_breaker_state` | `source` | gauge 0 closed, 1 half-open, 2 open |

`source` is the source id (bounded set, no secrets). Registered on the worker and preview registries.

## 7. Decisions
- Extend the existing `lookup` kind (requirement "step kind lookup") instead of a new kind; table modes unchanged.
- The port lives in `ctxhelpers` (no compile-pipeline change); drivers in `pkg/lookup`, never in `pkg/engine`.
- Cache is per compiled step (per worker, per route version, reset on reload); breaker/limits per source per process.
- Transient lookup failures apply the route error policy (not an in-memory retry loop like JS timeouts): the step
  already retried; blocking a partition on a down enrichment API would stall the route.
- Batching (MGET / IN per polled batch) is deferred: steps are per event today; noted as plan item PE-LK-30.
- Team guard in the gateway: every call carries the route's team (`RouteMeta.TeamID`); a spec of another team is
  refused (permanent). Routes validation reports the same case at save time (connections CN-LS-18).
- Mock answers are never cached; live answers in preview are cached per run (one compiled pipeline per run).
- API-side engine runs (DLQ dry-run/retry, saved test cases) use the in-process gateway (`cmd/api` `lookupGateway()`,
  wired at integration). Library publish checks only compile and keep `NoSourceGateway`.
- Integration: `lookup_transient` / `CategoryLookupTransient` were folded into the dead-letters contract
  (`lookup_unavailable`, `dependency_transient`); the engine-types schema enum lists `lookup_unavailable` and `lookup_miss`.
