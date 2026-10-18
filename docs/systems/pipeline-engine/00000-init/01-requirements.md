# Pipeline Engine — Requirements (00000-init)

## Business intent
One Kafka-free Go library runs the route config the user built: decode → filter → transform →
encode. Workers and the preview service embed the same code, which is what makes the promise
"what you see in preview is what runs in production" true (SysDesign §3, §7).
This folder also owns the route config document (SysDesign §5): its JSON Schema, generated
Go + TypeScript types, and the validation rules the API runs on Save draft and Deploy.

## Functional requirements
Route config document (`pkg/config`)
- FR-01 [MVP] One JSON Schema (`router/v1`) is the single source of truth for the route document (§5).
- FR-02 [MVP] Go types and TypeScript types (`web/src/api/generated`) are generated from the schema; CI fails on drift.
- FR-03 [MVP] Document-only validation in `pkg/config` returns issues (code, severity, JSON Pointer
  path, message) for: schema; JS/regex/formula compile (dry-run); `exactly_once` only when source and
  destination share a connection. External-data rules (connection access, destination topic, library
  refs, field types vs samples, test cases) run in `internal/routes/validation` (routes) and reuse the
  `IssueCode` enum defined here.
- FR-04 [MVP] `FieldUnknown` / `OperatorTypeMismatch` warning codes are defined here; the sample-based check is a routes validator.
- FR-05 [P2] `TestCaseFailed` code defined here; the rule (warning on Save draft, error on Deploy) runs in routes.
- FR-06 [MVP] Canonical JSON + SHA-256 `config_hash` so identical configs hash identically.
- FR-07 [P2] YAML ↔ JSON conversion of the same document (lossless) for import/export.
- FR-08 [MVP] Validation profiles per stage (draft, deploy, offline) are configurable, not hard-coded.
- FR-09 [MVP] `errorPolicy.dlqConnectionId` (optional) is part of the schema; default = source connection.

Event model and codecs (`pkg/engine`)
- FR-10 [MVP] Event = key, value, headers, topic, partition, offset, timestamp (§7.1).
- FR-11 [MVP] Codecs: JSON, string, bytes; decode once at entry, encode once at exit.
- FR-12 [P2] Avro and Protobuf via schema registry (Confluent wire format); schemas cached per ID.
- FR-13 [MVP] Decode/encode failures are typed errors (`decode`, `output_invalid`) for the runtime's DLQ policy.

Compile (`engine.Compile`)
- FR-20 [MVP] Compile once per route version: parse field paths into accessors, compile regexes,
  parse formulas, compile JS into sandbox modules, build filter tree and step chain.
- FR-21 [P2] Load inline lookup tables into maps. [P3] Resolve library transform/lookup refs by exact version.
- FR-22 [MVP] Every compile error carries the JSON Pointer of the offending node.

Filter engine
- FR-30 [MVP] JSON AST nodes `and`, `or`, `not`, `cond`, `expr`; lossless round-trip with the UI builder.
- FR-31 [MVP] All 19 `cond` operators of §7.3 evaluated natively with a fixed, documented coercion table.
- FR-32 [MVP] `expr` nodes run a one-line JS boolean in the sandbox.
- FR-33 [MVP] Short-circuit evaluation; trace records every evaluated node's result ("why filtered").

Transform engine
- FR-40 [MVP] Steps are `Event → []Event` (empty = drop, many = split); disabled steps are skipped.
- FR-41 [MVP] Core recipes: rename, remove, keep_only, copy, set (field/constant/metadata), move,
  mask, hash, convert_type, default_if_missing, drop_if, set_key; plus `js` step.
- FR-42 [P2] Remaining recipes: flatten, redact_pattern, format_date, round, string_case, lookup,
  if_else, split_array, wrap, unwrap, header ops (add/remove/rename/copy to/from payload).
- FR-43 [P2] Formula language (`concat(a," ",b)`, `round(x*1.1,2)`) parsed to AST, native function catalogue.
- FR-44 [P2] Visual Mapper `map` step: target tree with leaves field/constant/formula/metadata.
- FR-45 [P3] `library` step expands a pinned transform version inline at compile time.
- FR-46 [MVP] Recipes and JS `ctx` helpers share one implementation (mask, hash, now, formatDate, lookup).
- FR-47 [P2] Branches: each branch filters/transforms an isolated copy of the input; outputs carry branch id.

Tracing
- FR-50 [MVP] Optional trace per event: final status (passed/filtered/error/dropped), filter node
  results, per-step snapshots as RFC 6902 patches vs. the previous step, logs, timings, coercion warnings.
- FR-51 [MVP] Tracing off = zero snapshot cost (production path).

Tooling
- FR-60 [MVP] `routerctl run` CLI: run a route config over a JSONL file of events (milestone 1 deliverable).
- FR-61 [MVP] Static engine catalogue (operators per type, recipe kinds, formula functions) generated for the FE.

## Non-functional / constraints
- NFR-1 No Kafka, DB, Redis or network dependency in `pkg/engine` / `pkg/config` (ports only).
- NFR-2 Deterministic: frozen `now` per event, injectable clock; same input + config ⇒ same output.
- NFR-3 Never panics on any input (fuzzed); malformed bytes become typed errors.
- NFR-4 Throughput targets benchmarked in CI with regression thresholds: filter-only and recipe
  routes ≥ 100k events/s/core for 1 KB JSON (target, tune after first benchmark).
- NFR-5 Hard limits: max outputs per event (split fan-out), max output bytes; violations are step errors.
- NFR-6 Coding rules (.devlens/rules.md): base interface + Manager per extensible domain
  (codecs, filter nodes, operators, steps, formula functions, validators); registry selection, no type switches.
- NFR-7 Event payloads are never logged.

## Out of scope
- Kafka I/O, offsets, DLQ write, retries/backpressure → `route-runtime`.
- Sandbox internals, host-fn binding, limits enforcement → `js-sandbox`.
- HTTP endpoints (`/routes/{id}/validate`, `/preview/run`) → `routes`, `live-preview`.
- Transform-by-example / AI assist (produce config only) → `transform-studio`.
- Stateful processing (joins, windows, aggregations).

## Dependencies
- `js-sandbox`: `sandbox.Runtime`/`Module` for `js` steps and `expr` nodes; binds engine ctx helpers.
- `platform-core`: problem `path` format (JSON Pointer), secrets Manager for hash salt.
- `live-preview`: consumes trace JSON.
- `transform-studio`: library/lookup resolvers (FR-21, FR-45).
- `routes`: calls ValidatorManager (doc-only profiles), owns the external-data validators, uses the
  canonical hash and the YAML/JSON document format; maps FE draft ↔ generated types.
