# JS Sandbox — Requirements (00000-init)

Sources: SysDesign §7.5, §11 (sandbox hardening), §12 (sandbox metrics), §13 (sandbox security suite);
UI spec §8.5 (JS step contract), §8.8 (safety rules). Code: `pkg/sandbox` (+ thin `internal/sandboxinfo` handler).

## Business intent
Developers may write JavaScript inside a route (JS transform step, filter Expression mode).
Users and operators must trust that custom code cannot hurt the platform, leak data, reach other
routes, or behave differently in preview vs production. The sandbox is the single place where user
code runs, in workers and in the preview service alike.

## Functional requirements
- FR-1 [MVP] Run user JS in QuickJS compiled to WebAssembly, executed by wazero in-process (own QuickJS-ng
  build only if a wasi-sdk toolchain is available, otherwise an existing Go-embeddable QuickJS wasm module).
- FR-2 [MVP] Transform contract: user code defines `transform(event, ctx)`; result is an event,
  `null` (drop) or an array of events (split). Anything else is an `invalid_output` error.
- FR-3 [MVP] Predicate contract: a one-line boolean expression (filter `expr` node) evaluated with
  the same `event`/`ctx`; result coerced to boolean.
- FR-4 [MVP] `event` exposes `key, value, headers, topic, partition, offset, timestamp`; events cross
  the boundary as JSON-like values (objects, arrays, strings, numbers, booleans, null).
- FR-5 [MVP] `ctx` helpers are the ONLY host functions: `mask.{email,last4,full}`, `hash` (per-team
  salt injected by host), `now` (processing time, frozen per event), `formatDate`, `lookup(table, key)`, `log` (buffered,
  size-capped). Helpers share implementations with the equivalent recipe steps (identical output).
- FR-6 [MVP] Per-call limits: wall-clock timeout (default 50 ms), memory cap per instance (default
  64 MB), max output size, max log size, max stack depth. Exceeding one is a typed step error.
- FR-7 [MVP] Route limits come from route config `limits.jsTimeoutMs` / `jsMemoryMb`, clamped to
  operator maxima from service config.
- FR-8 [MVP] No network, filesystem, timers, environment, process or WASI access beyond stubs.
- FR-9 [MVP] Determinism: `Math.random` seeded per event, `Date.now()` / `new Date()` return the
  frozen `ctx.now()` value, so preview, saved test cases and production give repeatable results.
- FR-10 [MVP] One instance pool per route version; instances reused within that route, never shared
  between routes; every call gets a fresh JS context; an instance that timed out, ran out of memory or trapped is discarded.
- FR-11 [MVP] Check (compile) API: syntax errors and missing `transform` reported with line/column
  as validation issues at the step's JSON Pointer path (feeds "JS compiles in the sandbox" validation).
- FR-12 [MVP] Runtime errors carry message, line/column and stack so the UI can say
  "Event 7: Cannot read 'items' of undefined" on the exact line.
- FR-13 [MVP] Per-call result returns logs and duration for preview Console tab and step traces.
- FR-14 [MVP] Metrics: `router_js_timeouts_total`, `router_js_memory_bytes`, plus call duration,
  errors by kind and pool size, labelled by `route_id`, `version`.
- FR-15 [MVP] `GET /api/v1/sandbox/runtime-info`: effective limits, ctx helper catalogue, TypeScript
  declarations for Monaco, and the safety rules text shown in the JS editor info panel.
- FR-16 [MVP] Escape and fuzz test suite runs in CI and is a release gate.
- FR-17 [MVP] Benchmark in CI: typical 20-line transform < 0.5 ms/event (p50), regression threshold.
- FR-18 [P2] Faithful boundary marshalling for Avro/Protobuf values: int64 beyond 2^53, bytes,
  decimals, timestamps, using documented tagged representations.
- FR-19 [P2] Saved test cases run through the same sandbox with the same seed/now as preview;
  determinism contract versioned so stored expectations stay valid.
- FR-20 [P2] Branches: one pool per route shared across that route's branches and JS steps.
- FR-21 [P2] Per-route sandbox CPU-time and memory reporting for worker capacity scoring (§8.5).
- FR-22 [P3] Library transforms containing JS steps compile once per (code hash) within a route.
- FR-23 [P3] `ctx.lookup` resolves versioned library lookup tables (pinned version per route).
- FR-24 [P3] Pool pre-warm on hot reload so a new version serves its first event without cold start.

## Non-functional requirements / constraints
- NFR-1 Isolation is a security boundary: WASM linear memory, no host imports beyond `ctx` and
  deterministic WASI stubs; one bad route cannot affect others (§1 Isolation guarantee).
- NFR-2 Same code path in worker and preview (parity suite must pass).
- NFR-3 Instance creation < 5 ms with precompiled module cache; bytecode compiled once per version.
- NFR-4 Memory overhead per idle instance documented and bounded; pool size bounded by route
  parallelism (`maxInFlight`, partitions).
- NFR-5 Never log event payloads; `ctx.log` output only goes to preview/test results, capped.
- NFR-6 Follows `.devlens/rules.md`: base interfaces + Manager, config-selected engine, enums for
  kinds/limits/errors, class-based host functions, no magic numbers.
- NFR-7 Pure Go (no cgo); the QuickJS `.wasm` is embedded via `go:embed`.

## Out of scope
- Node/browser APIs, `require`/ES module imports, async/Promises across calls, timers.
- Stateful processing across events (module-level state is not a supported feature).
- Running JS in the API process for anything other than Check/validation.
- AI assist / Explain (transform-studio), error-policy decisions and DLQ writes (route-runtime).

## Dependencies on other folders
- docs/systems/pipeline-engine — JS step + `expr` node call the sandbox; owns route schema
  (`limits.*`), compile/validation wiring, trace types, and the shared mask/hash/date/lookup functions.
- docs/systems/route-runtime — maps sandbox errors to error policy/DLQ, owns pool lifecycle on
  hot reload and runner shutdown, retries timeouts up to `maxRetries`.
- docs/systems/platform-core — service config/env keys, metrics registry, secrets Manager (hash
  salt), `/api/v1` conventions (RFC 7807), migrations.
- docs/features/live-preview — returns per-event logs/errors/timings from sandbox results.
- docs/features/transform-studio — consumes runtime-info (Monaco typings, safety panel, snippets).
