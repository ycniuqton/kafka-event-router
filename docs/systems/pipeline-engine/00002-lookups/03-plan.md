# Pipeline Engine — Plan (00002-lookups)

Read `01-requirements.md` + `02-design.md`. Mark the item you start `[~]`, finished `[x]`; never delete items.
Source management (API, UI) is connections#00001-lookup-sources (`CN-LS-xx`).

## Config
- [x] PE-LK-01 Schema: `lookupSpec`, `lookupRequest`, `lookupMapping`, `lookupMissPolicy`, `lookupErrorPolicy`, `lookupCache` + enums; `lookup` kind rule `anyOf [field+rows, field+ref+version, lookup]` — done: `$defs` lookupSpec/lookupRequest/lookupMapping/lookupMissPolicy/lookupErrorPolicy/lookupCache/lookupTemplates + enums LookupRequestKind, LookupHTTPMethod, LookupRedisOp, LookupMissAction, LookupErrorAction
- [x] PE-LK-02 Regenerate `routeconfig_gen.go` (go-jsonschema) and web `routeConfig.ts` — done (generator names: `LookupHttpMethod`, `LookupRequestKindHttp`/`Sql`; Makefile capitalization flags unchanged)

## Engine (driver-free)
- [x] PE-LK-03 `ctxhelpers.SourceGateway` port + `Set.Sources` + `NoSourceGateway`, `SourceError` (Temporary) — done: `ctxhelpers/source.go`; `Set.Sources` defaulted by `WithDefaults`
- [x] PE-LK-04 `engine.ErrLookupTransient` (renamed `ErrLookupUnavailable` at integration, PE-LK-33) / `ErrLookupMiss` + `step.TypedError` hook in `ProcessErrors.ForStep` — done: `engine/errors.go` + `step.TypedError` checked first in `ProcessErrors.ForStep` (`chain.go`, shared file, 3 lines)
- [x] PE-LK-05 `step.FieldAccessor` / `FieldAccess` declaration (merge point with 00001-fast-codec) — done: `step/access.go` (`FieldAccess{Reads,Writes,FullTree}`, `FieldAccessOf` → FullTree for undeclared steps) — MERGE POINT with 00001-fast-codec
- [x] PE-LK-06 `enrich` templates (`${path}`, typed single placeholder, escaping) + SQL `:name` rewrite — done: `enrich/template.go`, `template_set.go` (maps, lists, JSON body), `sqlparams.go` (casts and quotes kept; `;`, `$n`, non-SELECT rejected)
- [x] PE-LK-07 `enrich` request compilers (http, redis, sql, static) + manager — done: `enrich/request.go` (manager, compile ctx, pointer tokens), `request_kinds.go`
- [x] PE-LK-08 Response mapping (`$` picks) + miss/error policies + manager — done: `enrich/mapping.go`, `policy.go` (`PolicyManager` keyed by action, `LookupFailure` typed error)
- [x] PE-LK-09 Answer cache (LRU, TTL, negative TTL, max entries) — done: `enrich/cache.go` (mutex LRU, values copied out)
- [x] PE-LK-10 `SourceLookup` step (timeout, retries, cache, policies, preview log line) + `recipes.Lookup` delegation — done: `enrich/step.go` + `compiler.go`; `recipes.Lookup` delegates when `lookup` is set; mock answers are never cached
- [x] PE-LK-11 Catalogue entry / docs for the lookup source mode (gen-catalog) — verified, no change: the engine catalogue lists step kinds + groups only (lookup already present); `gen-catalog` output identical. Request kinds/actions reach the web through the generated `routeConfig.ts` enums

## Runtime library `pkg/lookup`
- [x] PE-LK-12 Kinds, `SourceSpec`, client base + factory registry — done: `pkg/lookup/spec.go` (kinds, settings per kind, limits + defaults, `SourceSpec`), `secret.go` (`SecretBundle` + codec), `client.go` (`Client`/`ClientFactory`/`FactoryRegistry`, `ProbeError`, `Failures` classifier)
- [x] PE-LK-13 HTTP client (auth none/bearer/basic/header, TLS, 404 = miss, 5xx/429 transient) — done: `pkg/lookup/http.go` (`AuthAppliers` registry, TLS/CA, 1 MiB answer cap, JSON-or-text answers with the engine number policy)
- [x] PE-LK-14 Redis client (get/hget/mget, JSON decode) — done: `pkg/lookup/redis.go` (`RedisCommands` registry; LOADING/BUSY/… replies transient)
- [x] PE-LK-15 Postgres client (READ ONLY tx, statement_timeout, bound params, first row) — done: `pkg/lookup/postgres.go` (session `default_transaction_read_only=on` + `statement_timeout`, plus per call `BEGIN READ ONLY` + `SET LOCAL statement_timeout`; SQLSTATE classes 08/53/57/40 transient)
- [x] PE-LK-16 Static client (library lookup table rows) — done: `pkg/lookup/static.go` (rows loaded once via `TableLoader` = `config.LibraryResolver`; missing ref permanent)
- [x] PE-LK-17 Circuit breaker, limiter (concurrency + rate), guarded client — done: `pkg/lookup/breaker.go` (breaker + limiter), `guard.go` (only transient failures count; limiter refusals do not close a half-open breaker)
- [x] PE-LK-18 Gateway (spec provider, client cache with spec TTL) + remote spec provider — done: `pkg/lookup/gateway.go` (per-source slot, spec TTL 60 s, stale client kept on control-plane errors, replaced clients closed after 30 s); the remote provider lives in `internal/lookups/specclient` (needs the controlplane client)
- [x] PE-LK-19 Prometheus observer (`router_lookup_*`) — done: `pkg/lookup/metrics.go`
- [x] PE-LK-20 Preview gateway (mocks per step, live with capped timeout) + Tester — done: `pkg/lookup/preview.go` (`PreviewGateway` cap 2 s, `Tester` + `TestReports`)

## Wiring
- [x] PE-LK-21 (superseded by PE-LK-33) Runtime classifier: `lookup_transient` → `CategoryLookupTransient` → `apply_policy` — done: `domain.CategoryLookupTransient` → `DispositionApplyPolicy` (`failures/classifier.go`, `disposition.go`), `lookup_miss` → `step`; `failures/lookup_test.go`. DLQ `router.error_type` stays `step_error` (unmapped) — F8 owns `dlq_writer.go`/`record.go` mapping
- [x] PE-LK-22 Worker: `Helpers.Sources` = `lookup.Gateway` (remote specs, library tables, Prometheus) — done: `reconcile.EngineVersionCompiler.Sources` → `Helpers.Sources`; `cmd/worker/assembly.go` `lookupSources` (RemoteSpecs over the control-plane client, `DefaultFactories(library)`, `PromObserver` on the worker registry)
- [x] PE-LK-23 Preview: `RunRequest.lookupMocks` + `PreviewGateway` in `PipelineBuilder` — done: `RunRequest.lookupMocks` → `RunScope.LookupMocks` → `PipelineBuilder.options` wraps `PipelineBuilder.Sources` in `lookup.PreviewGateway`; `cmd/preview/assembly.go` builds the live gateway (control-plane specs, library tables, metrics)

## Tests
- [x] PE-LK-24 Template / SQL params / mapping unit tests — done: `enrich/template_test.go`
- [x] PE-LK-25 Cache + breaker + limiter unit tests — done: `pkg/lookup/gateway_test.go` (breaker cycle, limiter, gateway cache/refresh/stale/mismatch/unknown/breaker, metric series, preview mocks + cap, tester)
- [x] PE-LK-26 Client tests per kind with fakes (httptest, miniredis, fake pg querier, static rows) — done: `pkg/lookup/clients_test.go` (httptest, miniredis incl. auth, fake `PgRunner` + read-only pool config, fake table loader)
- [x] PE-LK-27 Step tests (hit, miss policies, error policies, retries, cache, typed errors, field access) — done: `enrich/step_test.go`, `cache_test.go` (outcomes, retries, timeout → lookup_unavailable (was lookup_transient), cache hit/miss + log modes, compile issue pointers, no gateway) with `enginetest.FakeSourceGateway`
- [x] PE-LK-28 Golden route example `lookup_source` (fake gateway) + preview handler test with mocks — done: golden `testdata/golden/lookup_source` (http hit/miss default/cache, redis flag, transient c9 → `lookup_unavailable` (was `lookup_transient`) in expected_errors, missing key field) via `enginetest.GoldenSources`; preview handler tests `internal/preview/httpapi/lookup_test.go` (live log line, mocks replace the call, no gateway → error)
- [x] PE-LK-29 Schema validation tests (valid/invalid lookup specs) — done: `pkg/config/schema_lookup_test.go`

## Deferred
- [ ] PE-LK-30 Batching within a polled batch (MGET / `IN` query) — deferred (nice-to-have): needs a batch-level pre-fetch hook in the runner

## Added during implementation
- [x] PE-LK-31 Team guard: `SourceCall.TeamID` (= `RouteMeta.TeamID`) vs `SourceSpec.teamId`; the gateway refuses another team's source as a permanent error (preview drafts cannot borrow other teams' sources)
- [x] PE-LK-32 Web fixture check: `internal/lookups/studio_fixture_test.go` validates and compiles `web/src/domain/routeConfig/testdata/lookup_source_steps.json`
- [x] PE-LK-33 Integration: one transient lookup error contract — `engine.ErrLookupUnavailable` ("lookup_unavailable") with `engine.DependencyError{Kind: lookup}` as cause (`enrich/policy.go`); classifier maps it to `dependency_transient` (retry-topic eligible); `CategoryLookupTransient` removed; DLQ writer emits `lookup_unavailable` (class transient), `lookup_miss` stays `step_error` (data); engine-types schema enum + `engineTypes.ts` regenerated; tests `failures/lookup_test.go`, `enrich/step_test.go`, golden `lookup_source`
- [x] PE-LK-34 Integration: API-side engine runs get the in-process gateway — DLQ `PipelineCache.WithSources` / `AssemblyDeps.Sources` (`cmd/api/wire_dlq.go`), test cases `RunnerDeps.Sources` (`cmd/api/wire_testcases.go`); tests `internal/dlq/pipeline_sources_test.go`, `internal/library/testcases/lookup_sources_test.go`
