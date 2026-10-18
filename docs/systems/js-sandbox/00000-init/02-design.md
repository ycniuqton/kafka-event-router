# JS Sandbox — Design (00000-init)

Scope: `pkg/sandbox` (runtime, pools, ctx host functions, limits, determinism, escape suite) and a
thin read-only HTTP handler `internal/sandboxinfo`. Callers: `pkg/engine` (JS step, filter `expr`
node — see docs/systems/pipeline-engine), workers (docs/systems/route-runtime) and the preview
service (docs/features/live-preview). The engine never touches wazero/QuickJS types directly.

## 1. Architecture

```
 pkg/engine (JS step / expr node)          internal/sandboxinfo (GET runtime-info)
            │ uses only                                   │ uses only
            ▼                                             ▼
   sandbox.Manager ────────────── EngineRegistry ── selects Runtime by EngineKind (config)
     │  Load(scope, sources, limits) → RouteSandbox
     │  Check(source, entry) → []Diagnostic
     │  Info() → RuntimeInfo
     ▼
   RouteSandbox (one per route version) ── Module per JS source ── Pool ── Instance (WASM)
                                                                         │
                       quickjs.Runtime (wazero) ◄────────────────────────┘
                         ├─ embedded quickjs.wasm (go:embed, precompiled, compilation cache)
                         ├─ host module "router_ctx": host_call → HostRegistry → HostFunction
                         └─ host module "wasi_snapshot_preview1": deterministic stubs only
```

## 2. Package layout and key types (Go)

| File | Types | Notes |
| --- | --- | --- |
| `pkg/sandbox/kinds.go` | `EngineKind` (`quickjs_wazero`), `EntryKind` (`transform`, `predicate`), `ErrorKind`, `ResetPolicy`, `HostFnName` | Enums only, no standalone constants |
| `pkg/sandbox/limits.go` | `Limits`, `LimitDefault`, `LimitPolicy` | Existing `Limits` extended with `MaxLogBytes`, `MaxStackBytes`, `MaxSourceBytes` |
| `pkg/sandbox/runtime.go` | `Runtime` (base interface), `Program`, `RuntimeFactory`, `EngineRegistry` | Existing `Runtime` reshaped (see plan) |
| `pkg/sandbox/manager.go` | `Manager` interface, `DefaultManager` | Only entry point for business logic |
| `pkg/sandbox/module.go` | `Module`, `RouteSandbox`, `Invocation`, `Result`, `RouteScope` | Existing `Module` reshaped |
| `pkg/sandbox/pool.go` | `Pool`, `Instance`, `PoolOptions`, `PoolStats`, `BoundedPool` | Per route version |
| `pkg/sandbox/errors.go` | `Error`, `Diagnostic`, `ErrorClassifier` | Typed step errors |
| `pkg/sandbox/boundary/` | `Marshaller` (base), `JSONMarshaller` [MVP], `TaggedMarshaller` [P2], `GuestEvent`, `Output` | JSON-like values across boundary |
| `pkg/sandbox/host/` | `HostFunction` (base), `HostRegistry`, `HostServices`, `MaskFn`, `HashFn`, `NowFn`, `FormatDateFn`, `LookupFn`, `LogFn`, `LogBuffer` | Class-based handlers |
| `pkg/sandbox/determinism/` | `Prelude` (embedded JS), `SeedPolicy`, `FrozenClock`, `DeterminismVersion` | Seeded RNG + Date override + lockdown |
| `pkg/sandbox/quickjs/` | `Runtime`, `instance`, `guestABI`, `wasiStubs` | Only concrete engine; registered by kind |
| `pkg/sandbox/quickjs/guest/` | `shim.c`, `build.sh`, `quickjs.wasm`, `quickjs.wasm.sha256` | Own QuickJS-ng build only if a wasi-sdk toolchain is available; otherwise an existing Go-embeddable QuickJS wasm module is vendored and `guestABI` adapts to its exports |
| `pkg/sandbox/observe.go` | `Observer` (base), `PromObserver`, `NopObserver` | Metrics behind interface |
| `pkg/sandbox/info.go` | `RuntimeInfo`, `HelperDoc`, `SafetyRule`, `TypingsBuilder` | Source of Monaco typings |
| `pkg/sandbox/escapetest/` | escape suite cases + runner | CI release gate |
| `internal/sandboxinfo/handler.go` | `Handler` | `GET /api/v1/sandbox/runtime-info`, `POST /check` [P2] |

### 2.1 Core interfaces

```go
type Runtime interface {                     // base class of the engine domain
    Kind() EngineKind
    Check(ctx context.Context, src Source) []Diagnostic
    Compile(ctx context.Context, src Source, lim Limits) (Program, error)
    Close(ctx context.Context) error
}
type Source struct { StepPath string; Entry EntryKind; Code string }  // StepPath = JSON Pointer in route doc
type Program interface {                     // compiled bytecode + limits, immutable
    NewInstance(ctx context.Context, host *host.HostRegistry) (Instance, error)
}
type Instance interface {
    Invoke(ctx context.Context, inv Invocation) (Result, error)
    MemoryBytes() int64
    Close(ctx context.Context) error
}
type Pool interface {
    Acquire(ctx context.Context) (Instance, error)
    Release(i Instance)                      // healthy → back to idle set
    Discard(ctx context.Context, i Instance) // timed out / OOM / trapped
    Stats() PoolStats
    Close(ctx context.Context) error
}
type Manager interface {
    Check(ctx context.Context, src Source) []Diagnostic
    Load(ctx context.Context, scope RouteScope, srcs []Source, req RouteLimits) (RouteSandbox, error)
    Info() RuntimeInfo
}
type RouteSandbox interface {
    Module(stepPath string) (Module, bool)
    Stats() []PoolStats
    Close(ctx context.Context) error         // drains pools; called by route-runtime
}
type Module interface {
    Call(ctx context.Context, inv Invocation) (Result, error)
}
type RouteScope struct { RouteID, VersionID string; TeamID string; Parallelism int; Mode ScopeMode } // Mode: worker|preview|test
type Invocation struct { Event boundary.GuestEvent; Now time.Time; Seed uint64 }
type Result struct {
    Outputs []boundary.Output // empty + Dropped=true for null; >1 for split; predicate → Bool
    Dropped bool; Bool bool
    Logs []string; LogsTruncated bool
    Duration time.Duration; MemoryBytes int64
}
type Error struct {
    Kind ErrorKind            // compile|runtime|timeout|memory|output_too_large|invalid_output|host|internal
    Message string; Line, Column int; Stack string
    Retryable bool            // true only for timeout (SysDesign §8.3)
}
```

### 2.2 Enums / domain constants

| Enum | Values |
| --- | --- |
| `EngineKind` | `quickjs_wazero` |
| `EntryKind` | `transform`, `predicate` |
| `ErrorKind` | `compile`, `runtime`, `timeout`, `memory`, `output_too_large`, `invalid_output`, `host`, `internal` |
| `ResetPolicy` | `fresh_context` (default), `lockdown_reuse` |
| `ScopeMode` | `worker`, `preview`, `test` |
| `HostFnName` | `mask.email`, `mask.last4`, `mask.full`, `hash`, `now`, `formatDate`, `lookup`, `log` |
| `LimitDefault` | timeout 50 ms, memory 64 MB, output 1 MB, log 16 KB/call, stack 256 KB, source 64 KB |
| `LimitMaximum` (operator caps, env-overridable) | timeout 1000 ms, memory 256 MB, output 8 MB, log 64 KB |
| `DeterminismVersion` | `1` (bump when prelude/seed semantics change; stored with test cases by transform-studio) |

### 2.3 Host functions (ctx)

`HostFunction` base: `Name() HostFnName; Invoke(call *HostCall) (json.RawMessage, error)`.
`HostCall` carries args (JSON), the frozen clock, the per-call `LogBuffer` and `RouteScope`.
`HostServices` is injected by the engine (implemented in pkg/engine, see pipeline-engine) so the
recipe steps and ctx helpers share one implementation and the sandbox never imports the engine:

```go
type HostServices interface {
    Masker() Masker                 // Mask(value, MaskStyle) string
    Hasher() Hasher                 // Hash(value) string, salt bound at Load from SaltProvider
    DateFormatter() DateFormatter   // Format(value, layout, tz) (string, error)
    Lookups() LookupResolver        // Lookup(table, key) (any, bool)
}
```

Guest side: the prelude defines `ctx` as a frozen object whose methods serialize args and call
the single import `router_ctx.host_call(fn_id, args_ptr, args_len) → result handle`. Host errors are
rethrown as JS `Error` with the host message (kind `host` if uncaught).

### 2.4 Boundary format

`GuestEvent` JSON: `{ key, value, headers: {name: string}, topic, partition, offset, timestamp }`
(`timestamp` ISO-8601 UTC string, `offset` number; header bytes that are not valid UTF-8 are passed
as `{"$bytes": "<base64>"}`). Output accepted: object with optional `key`, `value`, `headers`
(other fields ignored — topic/partition belong to the destination config), `null`, or an array of
such objects. P2 `TaggedMarshaller` adds `{"$long": "…"}`, `{"$bytes": …}`, `{"$decimal": …}`,
`{"$timestamp": …}` for Avro/Protobuf values; JSON-format routes keep `JSONMarshaller`.

### 2.5 Isolation model

- Each `Instance` = one wazero module instance with its own linear memory; one QuickJS `JSRuntime`.
- `wazero.RuntimeConfig`: `WithMemoryLimitPages(limit/64KiB)`, `WithCloseOnContextDone(true)`,
  shared `CompilationCache` (code only — no data shared between routes).
- QuickJS: `JS_SetMemoryLimit` slightly below the page cap (clean JS OOM error first),
  `JS_SetMaxStackSize`, no `quickjs-libc` (`std`/`os` modules absent), module loader disabled.
- WASI imports: custom stub module; `clock_time_get` → frozen clock, `random_get` → seeded PRNG,
  `fd_write` → discarded, `proc_exit` → trap; every other WASI function → `ENOSYS`. No preopens,
  no env, no args.
- `ResetPolicy=fresh_context`: a new `JSContext` per call, bytecode re-linked from the program
  (no state leaks between events). `lockdown_reuse`: intrinsics + `globalThis` frozen after load,
  context reused. Decision: a fresh context per call is the only production policy; `lockdown_reuse`
  stays a registered option for benchmarks only.
- Pools are per `RouteScope` (route version); never shared across routes or versions.

## 3. Data model touchpoints

- No new tables. Reads route config `limits.jsTimeoutMs`, `limits.jsMemoryMb` (schema owned by
  pipeline-engine); proposes new optional `limits.jsMaxOutputKb` in the schema.
- Hash salt (per team, decision): migration `0006_team_hash_salt.up.sql` adds `teams.hash_salt_ref text NULL`
  (secret ref resolved through `internal/secrets.Manager`; generated on first use). 0006 is fixed in the
  MVP migration order (after access-control 0005, before routes 0007).
- Lookup tables read through `LookupResolver` (engine loads `lookup_table_versions.rows`).
- `test_cases` unchanged; transform-studio stores `determinismVersion` inside `expected_output` meta.

## 4. Service configuration (env keys, added to internal/platform/config/keys.go)

| Key | Default | Purpose |
| --- | --- | --- |
| `ROUTER_SANDBOX_ENGINE` | `quickjs_wazero` | `EngineKind` selected via `EngineRegistry` |
| `ROUTER_SANDBOX_MAX_TIMEOUT_MS` | 1000 | Operator cap for `limits.jsTimeoutMs` |
| `ROUTER_SANDBOX_MAX_MEMORY_MB` | 256 | Operator cap for `limits.jsMemoryMb` |
| `ROUTER_SANDBOX_MAX_OUTPUT_KB` | 8192 | Operator cap for output size |
| `ROUTER_SANDBOX_RESET_POLICY` | `fresh_context` | `ResetPolicy` |
| `ROUTER_SANDBOX_POOL_MAX_IDLE` | 4 | Idle instances kept per pool |
| `ROUTER_SANDBOX_CACHE_DIR` | (tmp) | wazero compilation cache dir |
| `ROUTER_SANDBOX_SALTS` | `control_plane` | worker team-salt source (`control_plane` \| `process`) |
| `ROUTER_SANDBOX_PROGRAM_CACHE` | `route` | `ProgramCacheKind` (`route` \| `off`) [P3] |

## 5. API endpoints owned

All under `/api/v1`, RFC 7807 errors with `path` (platform-core conventions), any authenticated role.

`GET /api/v1/sandbox/runtime-info` [MVP] → 200
```json
{
  "engine": "quickjs_wazero",
  "determinismVersion": 1,
  "limits": {
    "defaults": { "jsTimeoutMs": 50, "jsMemoryMb": 64, "jsMaxOutputKb": 1024, "logKbPerCall": 16 },
    "maxima":   { "jsTimeoutMs": 1000, "jsMemoryMb": 256, "jsMaxOutputKb": 8192 }
  },
  "helpers": [ { "name": "mask.email", "signature": "(v: unknown) => string", "doc": "Masks an email like a***@x.com" } ],
  "typings": "interface RouterEvent { key: unknown; value: any; headers: Record<string,string>; ... } declare const ctx: RouterCtx;",
  "safetyRules": [ { "id": "isolation", "text": "Code runs in an isolated sandbox: no network, no file access, no access to credentials or other routes." } ]
}
```

`POST /api/v1/sandbox/check` [P2] — editor squiggles without a full preview run.
Request `{ "code": "function transform(e, ctx) {…}", "entry": "transform" }` →
200 `{ "diagnostics": [ { "severity": "error", "message": "unexpected token", "line": 3, "column": 7 } ] }`;
413 problem if code > `MaxSourceBytes`; 422 problem if `entry` unknown.

## 6. Flows

```
[HTTP] POST /api/v1/routes/{id}/validate (or Save draft / Deploy)   — owned by routes/pipeline-engine
└─ config.JSValidator (pipeline-engine) collects JS sources with StepPath
   └─ sandbox.Manager.Check(src)
      ├─ len(code) > MaxSourceBytes → Diagnostic{error, "source too large"}
      └─ Runtime.Check → scratch Instance (shared checker pool) → guest sb_check(code, entry)
         ├─ syntax error → Diagnostic{line, column, message} → Issue{path: StepPath+"/code", error}
         ├─ entry=transform and no function `transform` → Issue{error, "transform(event, ctx) not defined"}
         └─ ok → no issues
```
---
```
[event] worker runner start / hot reload for route version V   — triggered by route-runtime
└─ engine.Compile(config) (pipeline-engine)
   └─ sandbox.Manager.Load(scope{route, V, team, parallelism, worker}, sources, limits)
      ├─ LimitPolicy.Resolve(route limits, operator maxima) → clamped Limits
      │  └─ request above max → clamp + Issue{warning} returned to validator
      ├─ SaltProvider.ForTeam(team) → secrets.Manager.Get(teams.hash_salt_ref)
      │  └─ ref missing → generate salt, secrets.Manager.Put → teams.hash_salt_ref updated (audit by platform-core)
      ├─ for each source: Runtime.Compile(src) → Program (bytecode once per version)
      │  └─ compile error → Load fails → engine.Compile error → runner Error state (route-runtime)
      └─ BoundedPool per Program (max = parallelism, warm 1 instance) → RouteSandbox
```
---
```
[event] pipeline JS step / expr node processes one event   — per event, hot path
└─ Module.Call(inv{event, now=frozen, seed=SeedPolicy(route, topic, partition, offset)})
   ├─ Pool.Acquire
   │  ├─ idle instance → reuse
   │  ├─ none idle and size < max → Program.NewInstance
   │  └─ pool full → wait until released or ctx deadline → Error{internal, "pool exhausted"}
   ├─ Marshaller.ToGuest(event) → bytes written into guest memory
   ├─ context.WithTimeout(Limits.Timeout) → guest sb_call(entry, in)
   │  ├─ ctx.* used → [sync] host_call → HostRegistry → HostFunction.Invoke (see next block)
   │  ├─ deadline hit → wazero closes module → Error{timeout, Retryable} → Pool.Discard; router_js_timeouts_total++
   │  ├─ QuickJS OOM / memory pages exhausted → Error{memory} → Pool.Discard
   │  ├─ JS exception → Error{runtime, line, column, stack} → Pool.Release
   │  ├─ WASM trap → Error{internal} → Pool.Discard
   │  └─ returned → result length > MaxOutputBytes → Error{output_too_large} → Pool.Release
   ├─ Marshaller.FromGuest(result)
   │  ├─ null → Result{Dropped}
   │  ├─ object → Result{Outputs:[1]}
   │  ├─ array of objects → Result{Outputs:[n]} (split)
   │  └─ anything else → Error{invalid_output}
   ├─ attach LogBuffer contents (preview/test mode only; dropped in worker mode)
   └─ Observer.CallObserved(scope, kind, duration, memory) → metrics
      └─ engine maps Error → step error → error policy / DLQ (route-runtime)
```
---
```
[guest call] user code calls ctx.hash(v)
└─ prelude ctx.hash → JSON.stringify(args) → router_ctx.host_call(HASH, args)
   └─ HostRegistry.Get(HASH) → HashFn.Invoke
      ├─ args invalid → host error → rethrown in guest as JS Error (catchable by user)
      └─ ok → Hasher.Hash(value) with team salt → result JSON → guest returns string
```
---
```
[guest call] user code calls ctx.log(...)
└─ LogFn.Invoke → LogBuffer.Append(line)
   ├─ buffer + line ≤ MaxLogBytes → appended
   └─ exceeded → LogsTruncated=true, line discarded (no error)
```
---
```
[HTTP] GET /api/v1/sandbox/runtime-info   — Studio JS editor opens
└─ sandboxinfo.Handler → sandbox.Manager.Info()
   └─ TypingsBuilder renders d.ts from HostRegistry helper docs + GuestEvent shape → 200 JSON (cached, ETag)
```
---
```
[event] route version superseded / runner stop / preview session expiry
└─ RouteSandbox.Close
   ├─ pools stop handing out instances
   ├─ in-flight calls finish or hit their deadline
   └─ all instances closed → memory released → Observer pool gauges set to 0
```
---
```
[CI] make test-sandbox (release gate)
└─ escapetest.Runner runs every case against Manager with production limits
   ├─ case expectation met (timeout/memory/blocked/no-leak) → pass
   └─ otherwise → fail build
```

## 7. Determinism

- Prelude (runs before user code, then the global is locked): `Math.random` → sfc32 seeded from
  `Invocation.Seed`; `Date.now()` and `new Date()` without args → `Invocation.Now`; `ctx.now()` →
  same frozen ISO string. `Now` = processing time frozen per event in worker mode (decision), sample timestamp
  in preview/test.
- `SeedPolicy`: `fnv64a(routeID, topic, partition, offset)` — independent of version so a test case
  stays green across versions; preview of the same sample = same seed.
- `Intl` locale/timezone fixed to `en-US` / `UTC`.

## 8. Error cases

| Case | Kind | Retryable | Pool action | Surfaced as |
| --- | --- | --- | --- | --- |
| Syntax error / missing `transform` | `compile` | – | – | Validation issue at step path |
| Uncaught JS exception | `runtime` | no | release | Step error with line/col (preview inline) |
| Wall-clock exceeded | `timeout` | yes | discard | Step error; warning on step; DLQ after retries |
| Memory cap | `memory` | no | discard | Step error; `router_js_memory_bytes` |
| Output too big | `output_too_large` | no | release | Step error |
| Bad return type | `invalid_output` | no | release | Step error ("transform must return object, null or array") |
| ctx helper misuse uncaught | `host` | no | release | Step error |
| WASM trap / ABI fault | `internal` | no | discard | Step error + error log (no payload) |
| Pool exhausted until deadline | `internal` | yes | – | Backpressure signal to runner |
| Test pin from another determinism version [P2] | `determinism_mismatch` | no | – (no instance used) | Test case "re-record" |

## 9. Frontend touchpoints

- `web/src/domain/sandbox.ts`: `RuntimeInfo`, `HelperDoc`, `SafetyRule`, `Diagnostic` types.
- `web/src/api/contracts.ts`: `SandboxService` abstract class (`runtimeInfo()`, `check()` [P2]);
  mock in `web/src/api/mock/` registered in `web/src/api/registry.ts`.
- Monaco typings, info panel and snippets UI are owned by transform-studio; the existing
  `web/src/domain/pipeline/JsSandbox.ts` stays a mock-only fallback (not a security boundary).

## 10. Decisions

- Every call gets a fresh JS context (`ResetPolicy=fresh_context`). Module-level state never carries over.
- `ctx.now()` is the processing time, frozen per event, in worker mode. The record time is available as
  `event.timestamp`. A replay uses the replay processing time.
- The hash salt is per team (`teams.hash_salt_ref`, migration `0006_team_hash_salt`).
- QuickJS build: we build our own QuickJS-ng wasm (shim + `build.sh`) only if a wasi-sdk toolchain is
  available. Otherwise we vendor an existing Go-embeddable QuickJS wasm module and adapt `guestABI`. In
  both cases the WASI stubs, prelude and limits are the same.
- Issue paths are JSON Pointers (`/branches/0/transform/steps/2/code`).

### 10.1 MVP implementation notes

- Guest: no wasi-sdk toolchain was available, so `guest/quickjs.wasm` is the unmodified `qjs.wasm` of
  github.com/fastschema/qjs v0.0.6 (QuickJS-ng). `guestABI` drives its exports directly. The only
  imports are `env.jsFunctionProxy` (the ctx bridge) and our deterministic WASI stubs. No fastschema Go code is used.
- `fresh_context` means that each call starts from a restored snapshot of the instance's linear memory,
  taken after the program loaded. This is stronger than a new JSContext. A new JSContext per call cost
  about 3 ms in this build, while the snapshot restore costs about 20 µs. Top-level code runs once at load,
  with the clock frozen at the Unix epoch. If a call grows memory past the snapshot, the pool recycles that instance.
- Instance creation clones a boot template: the memory image of a guest after QuickJS and the prelude
  booted, one per memory cap. Linear memory is an anonymous mmap on Linux. Load plus instance takes about 1.6 ms.
- Instances evaluate the source, not bytecode. Bytecode is still compiled once per version as the syntax check.
  The vendored glue double-frees bytecode whose top level throws.
- The guest has no stack check: QuickJS-ng disables it on WASI. Deep recursion overflows the guest shadow
  stack. That traps the instance, which is discarded. The error kind is `stack_overflow`, a new `ErrorKind` that
  is not retryable. `MaxStackBytes` cannot be tuned with this build.
- Timeouts use wazero `CloseOnContextDone`, which roughly doubles call cost. The typical 20-line transform has
  p50 ≈ 0.38 ms. An own build (JS-09/10) with a QuickJS interrupt handler could remove this overhead.
- Runtime errors that the engine raises itself (for example "cannot read property of undefined") report the
  position of the last call site in that frame, not the exact token. This is a limitation of the vendored
  build. Explicit `throw` statements and syntax errors are exact.
- The engine owns the `script.Runtime` port and does not import `pkg/sandbox`. `pkg/sandbox/enginebind`
  implements that port on top of `Manager`. It also serves ctx helpers from the engine's `ctxhelpers.Set`,
  so recipe steps and ctx helpers share one implementation.
- In JS, headers are a map `{name: string | {$bytes}}`. The adapter converts this to and from the engine's
  `[{key, value}]` list.

- Team salts in workloads: the worker reads `GET /internal/v1/sandbox/teams/{teamId}/hash-salt` through
  `sandboxinfo.ControlPlaneSalts` (cached per team, failures not cached), selected by `ROUTER_SANDBOX_SALTS`
  (`control_plane` default, `process` stand-in). Preview (`ROUTER_PREVIEW_SALTS`) and API DLQ retry
  (`sandboxinfo.NewTeamSalts`) use the same salt, so `ctx.hash` is identical in worker, preview and retry.
- Metrics: the worker's `PromObserver` skips `router_js_timeouts_total` and `router_js_memory_bytes`
  (`sandbox.HostOwnedSeries`), which monitoring's route instruments export with runner labels. Preview
  keeps `NopObserver`, so preview runs never count as route traffic.
- FE: `services.sandbox.runtimeInfo()` drives the JS step badge, the Safety rules panel (rules, limits,
  helper list) and the Monaco extra lib. The browser `JsSandbox` remains a mock-mode stand-in; with the
  `http` data source, studio outputs must come from the preview service (transform-studio / live-preview).

### 10.2 P2 implementation notes

- Boundary (JS-101/102): `boundary.Registry` maps record formats to marshallers. `avro` and `protobuf` select
  `TaggedMarshaller`. Every other format uses `JSONMarshaller`. `RouteScope.Formats` carries the source,
  key and destination formats. The worker fills it from the route config (`enginebind.ScopeFormats`).
  Preview stays on JSON until it decodes Avro/Protobuf (live-preview LP-101). Tags: `{"$long"}` is used only
  outside ±(2^53-1), so smaller integers stay numbers. `{"$bytes"}` is base64, `{"$decimal"}` is an exact
  decimal string (`*big.Rat`), and `{"$timestamp"}` is RFC 3339 in UTC. On the way back, an object is read as
  a tag only if it has exactly one known `$` member with a string value. Exact integers become `int64`. A
  malformed tag is an `invalid_output` error. The typings declare `RouterLong`, `RouterBytes`,
  `RouterDecimal`, `RouterTimestamp` and `RouterTagged`.
- Shared pools (JS-103): `Manager.Open(scope, limits)` returns an empty `RouteSandbox`, and
  `RouteSandbox.Attach(src)` adds sources to it. Modules are keyed by `ModuleKey` = sha256(entry, code),
  which does not depend on the branch. Every step path aliases the module of its key, so identical JS steps
  and expr nodes in different branches share one pool. `enginebind.RouteScripts` is the engine's
  `script.Runtime` for one route version. It keeps one sandbox per limit set, counts module references and
  closes the sandbox after the last close. A double close is a no-op. Worker and preview use it.
  `ScriptRuntime` (one sandbox per source) remains for validation and DLQ retry.
- Accounting (JS-104): `PoolStats` adds `Calls`, `CPUTime` (guest `Invoke` time) and `MemoryBytes` for every
  live instance (last sampled size). `RouteSandbox.Usage()` and `Manager.Usage()` sum them per route version.
  `UsageSampler` turns these totals into `RouteLoad.CPUShare`, the CPU seconds per wall second since the
  previous sample. New metric: `router_js_cpu_seconds_total{route_id,version}`. `router_js_pool_instances`
  is now updated whenever the size of a pool changes. Feeding `RouteLoad` into the worker load score is
  route-runtime's step. The port is ready.
- Test pins (JS-105): `determinism.Pin{determinismVersion, seed, now}` is stored with a saved test case. It is
  passed as `Invocation.Pin` or through `determinism.PinContext` on the context, because the engine's
  script port only forwards `ctx`. Pins apply only in `ScopeMode=test`. If a pin's version differs from the
  running version, the call fails with `ErrorKind=determinism_mismatch` (not retryable). The test runner
  then reports the case as needing re-recording instead of failing it. `Result.Seed` and `Result.Determinism`
  return what a run used, so `determinism.Current(seed, now)` can record a pin.
- Check API (JS-106): `POST /api/v1/sandbox/check` needs `transform.read`. The body limit is 6 × MaxSourceBytes.
  A missing `entry` means `transform`, an unknown entry returns 422 at `/entry`, and code above MaxSourceBytes
  returns 413 at `/code`. If the engine is unavailable the endpoint returns 503. The API boots its sandbox
  lazily on the first check (`LazyChecker`, fixed `CheckSalts`), so API start-up never pays for the engine.
- FE (JS-107/108): `SandboxService.check(code, entry)` is backed by HTTP or by a parse-only `MockCodeChecker`.
  `MonacoSandboxCheck` waits 400 ms after typing stops, then puts the diagnostics on the code as markers
  (owner `js-sandbox-check`). Run-error markers keep their own owner. Answers for code that has changed since
  the request are dropped. A failed check clears the markers.
- Parity (JS-111): `internal/sandboxinfo/parity_test.go` runs the JSON golden set through the preview and the
  worker wiring. Both run under the preview route id. Preview runs do not carry the route id, so on the stack
  `Math.random` seeds differ between preview and worker. Fixing this needs a `routeId` on preview runs
  (live-preview follow-up). `ctx.hash`, outputs and errors already match.

```
[HTTP] POST /api/v1/sandbox/check   — Studio JS editor, 400 ms after typing stops
└─ sandboxinfo.CheckHandler
   ├─ body > 6 × MaxSourceBytes or code > MaxSourceBytes → 413 problem (path /code)
   ├─ entry not transform|predicate → 422 problem (path /entry)
   └─ LazyChecker.Check → [first call] sandbox.NewManager (engine boot)
      ├─ boot failed → 503 problem
      └─ Manager.Check → 200 {diagnostics} → FE sets Monaco markers (stale answers dropped)
```
---
```
[event] worker compiles route version V (branches a, b with identical JS step)
└─ engine.Compile → enginebind.RouteScripts.Compile(src per step)
   ├─ first source of a limit set → Manager.Open(scope{formats}) → RouteSandbox (marshaller by format)
   ├─ RouteSandbox.Attach(src)
   │  ├─ ModuleKey seen → alias step path → same pool (refs++)
   │  └─ new key → Runtime.Compile + BoundedPool → module (refs++)
   │     └─ compile error → sandbox closed if unused → CompileError
   └─ runner stop → each module Close → refs-- → last one → RouteSandbox.Close (pools drained)
```
---
```
[HTTP] saved test case run (transform-studio TS-106) in ScopeMode=test
└─ ctx = determinism.PinContext.With(ctx, stored pin) → engine run → Module.Call
   ├─ pin.Version != VersionCurrent → Error{determinism_mismatch} → case reported "re-record"
   └─ seed = pin.Seed, now = pin.Now → identical Math.random / Date / ctx.now() as when recorded
```

### 10.3 P3 implementation notes

- Program cache (JS-201): `ProgramCache` (base) with `RouteProgramCache` and `UncachedPrograms`, selected by
  `ROUTER_SANDBOX_PROGRAM_CACHE` through `ProgramCacheRegistry`. The key is `ProgramKey{routeId, ModuleKey, limits}`.
  The version and step path are not part of the key, so repeated library code and the next version of a route reuse
  one compiled `Program`. Pools and instances stay per version. Entries are ref-counted by modules and evicted with the
  last one. Concurrent compiles of one key wait for the first. Failures are never cached. `Manager.Programs()` reports
  entries, hits and compiles. Library expansion itself is pipeline-engine PE-100. Its JS steps arrive as ordinary sources.
- Pinned lookup tables (JS-202): `RouteScope.LookupPins` lists `{table, version}` from the route version's
  `lookup` steps with `ref` and `version` (`enginebind.ScopeLookups`, which includes nested if/else steps). At Open,
  `host.PinnedTablesLoader` loads every pin once through the `host.LibraryTables` port and keeps the rows immutable.
  It does this before any event, so later publications never change a running version. `ctx.lookup` asks the pinned
  tables first, then the engine's resolver. A missing pin, two versions of one table, or no library port
  (`ManagerDeps.Library` nil → `NoLibraryTables`) is a `compile` error. `enginebind.LibraryTables` adapts
  `config.LibraryResolver` (transform-studio `StoreLibraryResolver`, TS-207). Rows are normalised like the lookup recipe.
  Nothing implements that resolver yet, and the engine still rejects named lookup steps (PE-101). For now the worker
  passes pins without a library, so such versions fail to compile, as they already do in the engine. ctx.lookup
  arguments are capped (table 256 B, key 4 KiB) and throw a catchable host error. A missing table or key returns null.
- Pre-warm (JS-203): `Manager.Prewarm(scope)` warms every open sandbox of the route version (all limit sets) to
  `PrewarmTarget` = min(parallelism, `POOL_MAX_IDLE`), at least 1. Instances are booted outside the pool lock.
  `PoolStats.ColdStarts` / `RouteUsage.ColdStarts` count instances booted while a call waited. The worker's
  `EngineVersionCompiler` calls it through `ScriptProvider.Prewarm` right after the engine compile, which is before
  `Runners.Swap/Start`. Errors are ignored, because instances are still created lazily.
- Replay (JS-204): `determinism.Policy` (base) with `ProcessingPolicy`. The seed comes from `EventRef`
  (route, topic, partition, offset) for every `Pass`, and now is the processing time. A replay therefore draws the
  same `Math.random` values, and `ctx.now()` returns the replay time. `determinism.PassContext` lets route-runtime
  mark replay windows (RR-203).
- Host memory safety (found by JS-206): a timeout closes the module asynchronously, and the next guest call unmaps its
  mmap'd memory. Host code (bridge argv reads, ABI reads and writes) now checks `IsClosed` through `guestABI.memory()`
  before it touches guest memory. Before this fix, a deadline hit during a host call could SIGSEGV the process.

```
[event] worker compiles version V+1 while V runs (hot reload, RR-50)
└─ EngineVersionCompiler.Compile
   ├─ engine.Compile → RouteScripts → Manager.Open(scope{lookupPins}) → PinnedTablesLoader.Load
   │  ├─ pin missing / conflicting / no library → compile error → version fails (V keeps running)
   │  └─ Attach(src) → ProgramCache.Acquire(route, code, limits)
   │     ├─ cached (same code in V or elsewhere in V+1) → reuse Program (no compile)
   │     └─ new → Runtime.Compile → Program → BoundedPool (warm 1)
   ├─ ScriptProvider.Prewarm → Manager.Prewarm(scope V+1) → pools grow to min(parallelism, maxIdle)
   └─ Runners.Swap(V+1) → first events find idle instances (ColdStarts unchanged)
      └─ V closes → modules release Programs → last reference evicts the cache entry
```

## 11. Open questions (remaining)

1. Headers shape in JS: map `{name: string}` (proposal; duplicate keys → last wins) vs array.
