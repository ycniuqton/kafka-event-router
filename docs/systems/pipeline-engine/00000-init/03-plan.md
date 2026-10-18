# Pipeline Engine — Plan (00000-init)

Read 02-design.md first. Mark an item [~] when starting and [x] when done; never delete items.
Item ids (PE-xx) are referenced by other folders as `needs: pipeline-engine#PE-xx`.

## MVP

### BE — pkg/config
- [x] PE-01 Write `pkg/config/schema/route.v1.schema.json` (SysDesign §5 + additions in design §2: step `id`/`enabled`, startFrom, keyFormat, destination key/partitioning/headers/format/createTopic, limits, `errorPolicy.dlqConnectionId` (optional, default = source connection)) with `$defs/steps`, `$defs/filterNode`, `$defs/lookupRows`; embed via `go:embed` in `pkg/config/schema.go`
- [x] PE-02 Add `make gen-types` target: go-jsonschema → `pkg/config/routeconfig_gen.go`; keep enums in `pkg/config/types.go` and add missing enums (StartMode, FormatType alias, KeyMode, PartitioningMode, HeaderMode, MaskStyle, StepKind, NodeType, OpName, ValueSourceKind, MetadataName) — note: enum names come from titled `$defs` in the schema (generated, not hand-written in types.go); `types.go` keeps Severity, limit defaults and helpers. `make` is not installed in the build env, so the targets were checked by running the same commands
- [x] PE-03 `pkg/config/document.go`: `Document` (parsed RouteConfig + raw + JSON Pointer index) and `Parse(raw)`
- [x] PE-04 `pkg/config/issue.go`: `IssueCode` enum, add `Code` to `Issue`, `Issues` (sort by path, `HasErrors`, merge) — needs: platform-core#PC-14
- [x] PE-05 `pkg/config/validator.go` + `validator_manager.go`: change `Validator` signature to `Validate(ctx, *ValidationContext)`, `ValidationStage` enum, `ValidatorManager` built from a `ValidationProfile` config (stage → ordered validator names), registry by `ValidatorName`
- [x] PE-06 `pkg/config/ports.go`: `LibraryResolver` interface (compile-time refs) + in-memory fake in `pkg/config/configtest/`; external-data ports live in routes (`routes#m-be-validators`)
- [x] PE-07 `pkg/config/validators/schema.go`: JSON Schema validator (santhosh-tekuri/jsonschema v6), maps InstanceLocation → JSON Pointer, short-circuits later validators on failure
- [ ] PE-08 (invalidated by decision: external-data rule, implemented as routes#m-be-validator-context) — connection existence + team access validator; do not build in `pkg/config`
- [x] PE-09 `pkg/config/validators/delivery_mode.go`: exactly_once only when all branch destinations share source connection
- [ ] PE-10 (invalidated by decision: external-data rule, implemented as routes#m-be-validator-context) — destination topic validator; do not build in `pkg/config`
- [x] PE-11 `pkg/config/validators/compile.go`: dry-run `engine.Compile`, map `CompileError.Issues`, close pipeline (after PE-30)
- [ ] PE-12 (invalidated by decision: external-data rule, implemented as routes#m-be-validator-fieldtypes) — field-type warnings vs samples; keep only the `FieldUnknown`/`OperatorTypeMismatch` codes in PE-04
- [x] PE-13 `pkg/config/canonical.go`: RFC 8785 canonical JSON + SHA-256 `ConfigHash` for `route_versions.config_hash`
- [x] PE-14 `pkg/config/upgrade.go`: `Upgrader` interface + Manager keyed by `APIVersion`, v1 identity implementation

### BE — pkg/engine core
- [x] PE-20 `pkg/engine/errors.go`: `ErrorType` enum (decode, decode_transient, step, step_timeout, output_invalid, limit) + `ProcessError`
- [x] PE-21 `pkg/engine/path/`: `PathRoot` enum, `FieldPath.Parse` (`value.order.items[0].price`, `headers.traceId`), `Accessor` Get/Set/Delete/Exists on Event
- [x] PE-22 `pkg/engine/event.go`: add `RawRecord`, JSON (de)serialisation of Event matching `test_cases.input_event`, `Clone` (deep copy for branches)
- [x] PE-23 `pkg/engine/codec/`: replace `CodecManager` with `CodecFactory` + `FactoryManager` (registry); `Codec` gains ctx; JSONCodec (UseNumber → float64/int64 policy), StringCodec, BytesCodec
- [x] PE-24 `pkg/engine/ctxhelpers/`: `Set`, `Masker` (full/last4/email), `Hasher` (SHA-256 + per-team `SaltProvider` port, impl js-sandbox#JS-23), `Clock` (`SystemClock` = processing time frozen per event, `FixedClock`), `DateFormatter`, `LogSink` (size-capped) — needs: platform-core#PC-32 — note: `SaltProvider` port + `StaticSaltProvider`; the secrets-backed provider is js-sandbox#JS-23 (needs platform-core#PC-32)
- [x] PE-25 `pkg/engine/trace/`: `Tracer`, `NoopTracer`, `RecordingTracer`, `EventStatus` enum, `EventTrace` JSON shape (design §5), `PatchBuilder` (RFC 6902 via wI2L/jsondiff)
- [x] PE-26 `pkg/engine/filter/` nodes: `Node`, `NodeType`, `NodeCompilerManager`, and/or/not/cond with short-circuit + trace by node pointer
- [x] PE-27 `pkg/engine/filter/coerce.go`: `Coercer` implementing the fixed coercion table + `CoercionWarning` codes
- [x] PE-28 `pkg/engine/filter/operators/`: `Operator`/`Prepared`/`OperatorManager`; one file each for eq, ne, gt, gte, lt, lte, between, in, not_in, contains, starts_with, ends_with, regex (RE2), exists, not_exists, is_empty, before, after, within_last (uses Clock)
- [x] PE-29 `pkg/engine/filter/expr.go`: `expr` node wrapping code into a boolean module via `sandbox.Runtime` — needs: js-sandbox#JS-04, js-sandbox#JS-05 — note: built against the engine-local port `pkg/engine/script` (Runtime/Module, EntryPredicate) plus the `scripttest.FuncRuntime` fake. js-sandbox adapts `pkg/sandbox` to this port in js-sandbox#JS-28
- [x] PE-30 `pkg/engine/compile/`: `PipelineCompiler`, `CompileOptions`, `CompileError` collecting all issues with pointers, `compiledPipeline` (Decode/Process/Encode/Close) with single branch, limits (`maxOutputsPerEvent`, `maxOutputBytes`)
- [x] PE-31 `pkg/engine/step/`: `Step`, `StepKind`, `StepCompilerManager`, `StepChain` (drop/split semantics, disabled steps removed at compile, per-step trace patch)
- [x] PE-32 Recipes batch A in `pkg/engine/step/recipes/`: rename, remove, keep_only, copy, move
- [x] PE-33 Recipes batch B: set (field/constant/metadata value sources), default_if_missing, set_key
- [x] PE-34 Recipes batch C: mask, hash (via ctxhelpers), convert_type (`onFailure` error|null|keep), drop_if (reuses filter node compiler)
- [x] PE-35 `pkg/engine/step/js.go`: JS step compiled via sandbox, result null → drop, array → split, logs → tracer — needs: js-sandbox#JS-20 — note: built against the engine-local `pkg/engine/script` port (EntryTransform); the production adapter is js-sandbox#JS-28. routerctl takes `--js-engine` from `CLI.ScriptRuntimes` (empty until js-sandbox registers one)
- [x] PE-36 Wire `ValidatorManager` default profiles (draft, deploy, offline) with the doc-only validators (schema, delivery_mode, compile) in `pkg/config/profiles.go`

### BE — tooling
- [x] PE-40 `cmd/routerctl/`: `run` command (design §6.8) with `--config --events --trace --now --library`; exit codes as domain enum
- [x] PE-41 `pkg/engine/catalog/` + `cmd/gen-catalog/`: emit `web/src/api/generated/engineCatalog.json` (operators by field type, coercion table, recipe kinds/groups, mask styles)
- [x] PE-42 Makefile: `gen` (gen-types + TS types + catalog) and `check-gen` (regenerate to scratch dir and diff, CI-failing) — note: verified by running the target commands directly (`make` is not installed in this env). MVP-phase re-check: the gen targets had been lost from the shared Makefile and were restored (`gen`, `gen-types`, `gen-ts`, `gen-catalog`, `check-gen`, `engine-bench-gate`); every generated file was reproduced byte-for-byte with the target commands

### FE — generated types
- [x] PE-45 Add `json-schema-to-typescript` dev dep and `gen:types` script in `web/package.json` → `web/src/api/generated/routeConfig.ts` (header "generated, do not edit") — note: web/package.json is outside this folder's ownership, so the Makefile `gen-ts` target runs the pinned `npx --yes json-schema-to-typescript@15.0.4` instead of a dev dep/script. `cmd/gen-tsschema` first strips if/then conditionals, which json2ts cannot type. Also emits `engineTypes.ts` (EventTrace, ValidationIssue) from `schema/engine-types.v1.schema.json`
- [x] PE-46 `web/src/api/generated/index.ts` re-exporting `RouteConfig`, `FilterNode`, `Step`, `EventTrace`, `ValidationIssue` types and typed `engineCatalog` import (mapping FE draft ↔ RouteConfig is routes' job — needs: routes#m-fe-draft-mapper) — note: also emits `engineCatalog.ts` (typed TS module next to the JSON, because web tsconfig has no resolveJsonModule) and a hand-written `engineCatalogTypes.ts`. MVP-phase: `engineCatalogContract.test.ts` (vitest) checks that builder operators, Studio step kinds and mask modes all exist in the generated catalogue

### Tests
- [x] PE-50 Unit: schema accepts §5 example; rejects unknown kind/op/type with correct pointer (`pkg/config/schema_test.go`)
- [x] PE-51 Unit: each validator with fakes from `configtest/` incl. draft vs deploy severities
- [x] PE-52 Unit: canonical hash stable across key order / whitespace
- [x] PE-53 Unit: FieldPath parser + Accessor table tests; `FuzzFieldPath`
- [x] PE-54 Unit: every operator × coercion row table (`operators_test.go`); `FuzzFilterASTRoundTrip` (JSON → Node tree → JSON lossless)
- [x] PE-55 Unit: every MVP recipe (missing field, strict, nested, arrays); `FuzzRecipesNoPanic`
- [x] PE-56 Unit: JSON/string/bytes codecs; `FuzzJSONCodec` malformed bytes never panic
- [x] PE-57 Unit: trace — filtered event records failing node; patches apply back to reproduce each step output
- [x] PE-58 Golden harness `pkg/engine/golden_test.go` + ≥ 10 cases in `pkg/engine/testdata/golden/` (filter-only, mask+rename, js split, drop, coercion warning, decode error) — 14 cases; `js_split` uses fake scripts from `enginetest`
- [x] PE-59 Benchmarks `BenchmarkFilterOnly`, `BenchmarkRecipeChain` with thresholds file `pkg/engine/testdata/bench_thresholds.json` checked in CI — note: measured ~23–28k events/s per core for 1 KB JSON on this host (Xeon 6148). The cost is in the codec (JSON decode/encode ≈ 85%) after a move to a reflection-free jsontext tree codec. Thresholds are set at about 50% of measured and the gate runs with `ROUTER_BENCH_GATE=1`. The 100k/s target in NFR-4 is not met yet and needs a follow-up on codec tuning
- [x] PE-60 CLI test: `cmd/routerctl` runs a golden case and matches expected output

## P2

### BE
- [x] PE-70 Schema + engine: multiple branches — copy-on-write `Clone` per branch, outputs tagged, status aggregation (design §6.3) — done: schema `branches` 1–16 + unique ids (compile issue), last branch takes the original event, others a deep `Clone`; golden `branches` (+ trace); verified multi-branch preview on the local stack
- [x] PE-71 `pkg/engine/codec/registry.go`: `SchemaRegistry` port + HTTP client per connection + LRU `SchemaCache` by schema ID — needs: connections#CN-102 — done: `codec/registry.go` (port, `RegistryProvider`, `ConnectionRegistry`), `registry_http.go` (Confluent REST client, basic/bearer, `HTTPRegistryProvider` over a `RegistryConfigSource` port), `cache.go` (`LRU`, `CachedRegistry`); wired as `CompileOptions.SchemaRegistries`. The per-connection settings source is connections#CN-102 (not built yet), so bootstraps pass no provider and avro/protobuf records fail as `decode` until it lands
- [x] PE-72 `pkg/engine/codec/avro.go` (hamba/avro v2): Confluent framing, logical types, encode by subject; `ErrDecodeTransient` for registry outages — done: `avro.go` + `avro_values.go` + `avro_union.go` (schema-guided mapping, references, allocation limits); destinations default the subject to `<topic>-value`/`<topic>-key`; transient registry errors → `decode_transient`
- [x] PE-73 `pkg/engine/codec/protobuf.go`: Confluent framing incl. message indexes, dynamic messages → map — done: `protobuf.go` + `protobuf_values.go` (protocompile + dynamicpb, references as imports, Timestamp WKT, recover on runtime panics)
- [x] PE-74 `pkg/engine/formula/`: lexer, parser, AST, `Evaluator`, `FunctionCatalog` (concat, upper, lower, trim, substring, len, round, coalesce, if, toNumber, toString, now, formatDate, mask, hash, lookup) — done: lexer, precedence-climbing parser, AST, operator registry, `FunctionCatalog` with lazy `Args` (all 16 functions); `lookup()` reads inline tables by lookup step id
- [x] PE-75 `set` step `from: formula` value source using formula package — done: `step.FormulaSource`; `ValueResolver.Resolve` now also returns an error (`formula_failed`)
- [x] PE-76 Recipes batch D: flatten, redact_pattern (credit card, phone presets as enum), format_date (tz), round, string_case — done: one file per recipe; `FieldTransformBuilder` shares field/to/missing/null handling; presets credit_card, phone, email, iban
- [x] PE-77 Recipes batch E: lookup (inline rows → map, default value), if_else (filter condition + nested steps), split_array, wrap, unwrap — done: inline `rows` only (named `ref` tables = P3 → `library_ref_missing`); `if_else` uses `thenSteps`/`elseSteps` (nested chain via `StepCompileCtx.Chain`); wrap/unwrap restricted to key/value member paths
- [x] PE-78 Recipes batch F: header ops (add, remove, rename, copy_to_payload, copy_from_payload) — done: `header_add` (value source, set semantics), `header_remove`, `header_rename` (`newHeader`), `header_copy_to_payload`, `header_copy_from_payload` in `recipes/header_ops.go`
- [x] PE-79 `pkg/engine/step/mapper.go`: `map` step — target tree compiled to native accessors, start modes copy_input/empty, leaves field/constant/formula/metadata — done: `step.MapperCompiler` + `MapTree`; schema `$defs/mapField` tree under `value`, leaves = value sources or `remove: true`; sources resolve against the input before the output is built
- [ ] PE-80 (invalidated by decision: external-data rule, implemented as routes#p2-be-testcase-validator) — keep only the `TestCaseFailed` code in PE-04
- [x] PE-81 `pkg/config/docformat.go`: `DocumentFormat` Manager with JSON and YAML (lossless, key order preserved); consumed by routes#p2-be-codecs — done: `pkg/config/docformat.go` (`DocumentFormatManager`, `JSONDocFormat`, `YAMLDocFormat` on yaml.Node, order + number text preserved)
- [x] PE-82 Extend `engineCatalog.json` with formula functions + signatures and P2 recipes — done: catalogue v2 (formula functions with call form/arity/params, operators, presets, cases, map starts, formats, groups `structure`/`kafka`)

### FE
- [x] PE-85 Regenerate `routeConfig.ts` for branches/map/formula/lookup schema additions; publish `formulaFunctions` from catalog for Studio autocomplete (consumed by transform-studio#TS-118) — done: regenerated `routeConfig.ts` (json2ts now with `--maxItems -1`, Makefile updated) + Go types; `engineCatalogTypes.ts` and `index.ts` export `formulaFunctions` and the new types; vitest contract covers them

### Tests
- [x] PE-90 Unit: Avro/Protobuf codecs with fake registry; `FuzzAvroDecode` — done: `avro_test.go`, `protobuf_test.go` with `codectest.MemoryRegistry`, HTTP registry test (httptest), `FuzzAvroDecode` + `FuzzProtobufDecode` (found and fixed: truncated Avro accepted as null, unbounded array allocation, protobuf runtime panic, nullable 3-member unions, years > 9999)
- [x] PE-91 Unit: formula parser/evaluator table tests; `FuzzFormulaParser` — done: `formula_test.go` table tests (eval, positions of syntax errors, runtime errors, limits, catalogue examples) + `FuzzFormulaParser`
- [x] PE-92 Unit: P2 recipes + mapper; extend `FuzzRecipesNoPanic` to all kinds — done: `recipes_p2_test.go` (batches D/E/F, mapper, compile issue pointers, nested error ids); `FuzzRecipesNoPanic` covers every kind
- [x] PE-93 Golden: branches, mapper, lookup, if_else, split_array, Avro round-trip cases — done: goldens `branches`, `mapper`, `lookup_formula`, `if_else`, `split_array`, `header_ops`, `avro_roundtrip` (harness `registry.json` → fake registry + JSON↔wire adapters in `enginetest/wire.go`, `batch.Runner.Input/Output`)
- [x] PE-94 Unit: YAML ↔ JSON round-trip on every golden route; `BenchmarkMapper` — done: `TestYAMLRoundTripGoldenRoutes` (byte-identical + same config hash on every golden route); `BenchmarkMapper` (~18k events/s here) with a 9000 events/s gate threshold
- [x] PE-95 Integration: Avro codec against real schema registry (Testcontainers) — needs: platform-core#PC-54 — written, not run here: `codec/registry_integration_test.go` against a Confluent-compatible registry via `internal/testkit` (new `ServiceSchemaRegistry`, `ROUTER_TEST_SCHEMA_REGISTRY_URL`, container image apicurio-registry-mem); skips without Docker/registry (none in this env)

## P3

### BE
- [x] PE-100 `library` step: `LibraryResolver.Transform(ref, version)` spliced inline at compile; step ids namespaced `<libRef>@v/<stepId>` in trace — needs: transform-studio#TS-208 — done (engine side; design §12.1): `recipes.Library` + `step.Splicer` splice the pinned steps into the parent chain; `compilectx.LibraryCache` loads each pin once per compile; nested issues re-reported at the library step; cycles and depth > 4 refused. Deviation: trace ids are `<stepId>/<ref>@<version>/<childId>` (library step id kept as prefix). The production resolver is transform-studio#TS-208 (not built yet): no service passes `CompileOptions.Library`, so on the local stack library steps validate to `library_ref_missing` ("no library is available")
- [x] PE-101 `lookup` step + `ctx.lookup` with `ref`+`version` to library lookup tables via `LibraryResolver.LookupTable`; `ctxhelpers.LookupStore` backed by compiled maps — done (engine side; design §12.2): lookup `ref`+`version` → `LibraryResolver.LookupTable`; tables registered as `<stepId>`, `<ref>@<version>` and the alias `<ref>` (dropped when ambiguous) in `TableLookupStore`, shared by recipe, formula `lookup()` and `ctx.lookup` through the new `script.Source.Lookups` (bound in `compilectx.Scope.Script`). The QuickJS adapter reading `Source.Lookups` is js-sandbox#JS-202
- [x] PE-102 (invalidated by decision: external-data rule, implemented as routes#p3-be-validator-library) — keep `LibraryRefMissing` (compile, PE-100) and `LibraryUpdateAvailable` codes in the `IssueCode` enum — verified: both codes stay in `pkg/config/issue.go`; `library_ref_missing` is now emitted by PE-100/PE-101 — integration P3: closed as [x] (invalidated by decision, both codes verified).
- [x] PE-103 Engine support for replay/exactly-once: none beyond typed errors; confirm no engine change needed — needs: route-runtime#RR-201 — done: confirmed no engine change; `TestReplayIsDeterministic` (same record → identical outputs across attempts and recompiles, incl. library pins, formula now() and metadata); design §12.3

### FE
- [x] PE-105 Regenerate types for `library` step and lookup refs; expose catalogue entries for library step — done: schema documents `ref`/`version`, Go/TS regenerated (shape unchanged, no drift); catalogue v3 `library` entry; `index.ts` exports `libraryCatalog`, `CatalogLibrary`, `LibraryStepIds` (+ vitest `libraryStepIds.test.ts`)

### Tests
- [x] PE-110 Unit + golden: library step expansion, version pinning, deleted version → compile error — done: `compile/library_test.go` (expansion + spliced trace ids, version pinning v1/v2, deleted version / unknown ref / no resolver / store down / cycle / nested issue → issue paths, nested library + inside if_else, one load per pin, table names), golden `library_transform` (+ trace, `library/` dir via `configtest.LibraryDir`), routerctl golden + missing-version test
- [x] PE-111 Golden: lookup via library table and via `ctx.lookup` in JS give identical output — needs: js-sandbox#JS-202 — done with the engine fake: golden `library_lookup` (recipe branch vs JS branch through `ctx.lookup` on `Source.Lookups`) + `TestLibraryLookupParity`; the real QuickJS binding follows js-sandbox#JS-202
- [x] PE-112 Release gate: golden + fuzz (60 s per target) + bench thresholds wired into CI `make engine-gate` — done: Makefile `engine-gate` (go test config/engine/routerctl incl. goldens, 7 engine fuzz targets × `ENGINE_FUZZTIME`=60s, `engine-bench-gate`) + `.github/workflows/engine-gate.yml` (with `check-gen`). `make` is not installed here: the same commands were run directly, all fuzz targets 60 s each passed

### Acceptance fixes (2026-10-16, test/acceptance)
- [x] PE-AF-01 Email mask keeps the first letter of the local part (`john@mail.com` → `j***@mail.com`, UI spec §8.4; found by TC-TS-02) — `ctxhelpers/mask.go` `MaskPolicy.EmailVisibleHead` (1-char local parts stay fully masked), unit cases in `ctxhelpers_test.go` / `recipes_test.go`, goldens regenerated (engine + preview), sandbox helper doc and web mock typings updated
