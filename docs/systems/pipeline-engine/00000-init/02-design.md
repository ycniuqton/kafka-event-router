# Pipeline Engine — Design (00000-init)

Scope: `pkg/config` (route document, schema, validation, hashing) and `pkg/engine` (event model,
codecs, compile, filter, recipes, formulas, mapper, tracing), plus generated TS types/catalogue for `web/`.
Source specs: SysDesign §5, §7.1–7.4, §13, §14; UI spec §7.2, §8.3–8.5, §9.

## 1. Package layout

```
pkg/config/
  schema/route.v1.schema.json     source of truth (go:embed)
  types.go                        hand-written enums (exists: APIVersion, DeliveryMode, ErrorAction, Severity)
  routeconfig_gen.go              generated structs (go-jsonschema)  — never edited by hand
  document.go                     Document = parsed RouteConfig + raw bytes + pointer index
  issue.go                        Issue, IssueCode enum, Issues (sort/merge/hasErrors)
  validator.go                    Validator interface, ValidationStage enum, ValidationContext
  validator_manager.go            ValidatorManager (profile → ordered validators)
  validators/                     doc-only rule families: schema, delivery_mode, compile
                                  (external-data rules live in internal/routes/validation)
  ports.go                        LibraryResolver (compile-time library/lookup refs, P3)
  canonical.go                    Canonicalizer (RFC 8785 JCS) + Hasher → config_hash
  docformat.go                    DocumentFormat (json | yaml) Manager — P2
  upgrade.go                      Upgrader interface keyed by APIVersion (v1 identity only)
pkg/engine/
  event.go, codec.go              exist; extended below
  errors.go                       ErrorType enum, ProcessError
  path/                           FieldPath parser, Accessor (get/set/delete/exists), PathRoot enum
  codec/                          JSONCodec, StringCodec, BytesCodec, AvroCodec, ProtobufCodec,
                                  CodecFactoryManager, SchemaRegistry port, SchemaCache
  filter/                         Node interface, NodeType enum, NodeCompilerManager,
                                  and.go/or.go/not.go/cond.go/expr.go, Operator interface,
                                  OperatorManager, operators/*.go (one per op), Coercer
  step/                           Step interface, StepKind enum, StepCompilerManager,
                                  recipes/*.go (one per recipe), js.go, mapper.go, library.go
  formula/                        lexer, parser, AST, Evaluator, Function interface, FunctionCatalog
  ctxhelpers/                     Masker, Hasher, Clock, DateFormatter, LookupStore (shared with js-sandbox)
  trace/                          Tracer interface, NoopTracer, RecordingTracer, EventTrace, PatchBuilder
  compile/                        PipelineCompiler, CompileContext, RefResolver, compiledPipeline, branch
  catalog/                        Catalog builder (operators, recipes, functions) for FE generation
  testdata/golden/<case>/         route.json, input.jsonl, expected.jsonl
cmd/routerctl/                    CLI: `routerctl run --config r.json --events in.jsonl [--trace]`
cmd/gen-catalog/                  writes web/src/api/generated/engineCatalog.json
web/src/api/generated/            routeConfig.ts (json-schema-to-typescript), engineCatalog.json
```

Dependency direction: `config` ← `engine` → `sandbox` (interface) ; `sandbox` → `engine/ctxhelpers`.
`ctxhelpers` imports nothing from `engine`, so there is no cycle.

## 2. Route document (JSON Schema `router/v1`)

Follows SysDesign §5 exactly, with these additions needed by the UI (FE already models them):

| Path | Type | Notes |
| --- | --- | --- |
| `source.startFrom` | `{mode: latest\|earliest\|timestamp\|offsets, at?, offsets?{partition:offset}}` | UI §7.1 |
| `source.format` / `source.keyFormat` | `{type: json\|avro\|protobuf\|string\|bytes, subject?, messageType?}` | keyFormat default `string` |
| `branches[].filter` | Filter AST (null = pass all) | |
| `branches[].transform.steps[].id` | string, unique per branch | stable id for trace + UI (new) |
| `branches[].transform.steps[].enabled` | bool, default true | Studio toggle (new) |
| `branches[].destination.key` | `{mode: keep\|field\|none, field?}` | |
| `branches[].destination.partitioning` | `same_as_source\|by_key\|round_robin` | |
| `branches[].destination.headers` | `{mode: keep_all\|keep_selected\|drop_all, keep?[], add?[{key,value}]}` | |
| `branches[].destination.format` | `{type: same_as_source\|json\|avro, subject?}` | |
| `branches[].destination.createTopic` | `{partitions, retentionMs}?` | |
| `errorPolicy` | `{onError: dlq\|skip\|pause, dlqTopic?, dlqConnectionId?, maxRetries}` | `dlqConnectionId` added here (decision); absent = source connection |
| `limits` | `{jsTimeoutMs, jsMemoryMb, maxInFlight, maxOutputsPerEvent, maxOutputBytes}` | last two new |

Filter AST node shapes:
```
{type:"and"|"or", children:[Node]}      {type:"not", child:Node}
{type:"cond", field, op, value?, values?[], from?, to?, amount?, unit?}
{type:"expr", code:"event.value.amount > 100"}
```
Node identity for trace = JSON Pointer of the node within the branch filter (`/children/1`).

Step shapes: `{id, kind, enabled, ...kind-specific}`; kind-specific objects are a `oneOf`
discriminated by `kind`. Value sources (set step, mapper leaves):
`{from:"field", path}` · `{from:"constant", value}` · `{from:"formula", expr}` · `{from:"metadata", name: topic|partition|offset|timestamp|route_name|route_version}`.

Generation: `make gen-types` runs go-jsonschema → `routeconfig_gen.go` and
`npm --prefix web run gen:types` (json-schema-to-typescript) → `routeConfig.ts`; `make check-gen`
regenerates and fails CI on `git diff`-style drift (compare to temp dir, no VCS required).

## 3. Key types and interfaces

```go
// pkg/config
type ValidationStage string            // StageDraft, StageDeploy
type IssueCode string                  // SchemaInvalid, ConnectionNotFound, ConnectionForbidden,
                                       // FieldUnknown, OperatorTypeMismatch, PathInvalid, RegexInvalid,
                                       // FormulaInvalid, JSCompile, LibraryRefMissing, LibraryUpdateAvailable, TopicMissing,
                                       // ExactlyOnceCrossCluster, TestCaseFailed, LimitOutOfRange
type Issue struct { Code IssueCode; Path string; Message string; Severity Severity }
type ValidationContext struct { Stage ValidationStage; TeamID string; Doc *Document }
type Validator interface { Name() ValidatorName; Validate(ctx context.Context, vc *ValidationContext) []Issue }
type ValidatorManager struct { profiles map[ValidationStage][]Validator }  // built from ValidationProfile config
func (m *ValidatorManager) Validate(ctx context.Context, stage ValidationStage, teamID string, raw []byte) Issues

type LibraryResolver    interface { Transform(ctx, ref string, ver int) (Steps, error);
                                    LookupTable(ctx, ref string, ver int) (Rows, error) }        // transform-studio (P3)
// ConnectionResolver, TopicInspector, SampleSchemaProvider, TestCaseRunner ports belong to
// internal/routes/validation (external-data rules, owned by routes); they emit this IssueCode enum.
type Canonicalizer struct{}; func (Canonicalizer) Hash(raw []byte) (ConfigHash, error)
```

The existing `Validator.Validate(ctx, doc []byte)` signature changes to take `*ValidationContext`
so the document is parsed once; `Issue` gains `Code`.

```go
// pkg/engine
type RawRecord struct { Key, Value []byte; Headers []Header; Topic string; Partition int32; Offset int64; Timestamp time.Time }
type ErrorType string                  // ErrDecode, ErrDecodeTransient, ErrStep, ErrStepTimeout, ErrOutputInvalid, ErrLimit
type ProcessError struct { Type ErrorType; Branch BranchID; StepID string; Path string; Cause error }

type Pipeline interface {                                  // replaces current single-method interface
    Decode(ctx context.Context, r RawRecord) (Event, error)
    Process(ctx context.Context, in Event, tr trace.Tracer) ([]Out, error)
    Encode(ctx context.Context, o Out) (RawRecord, error)
    Close(ctx context.Context) error                       // releases sandbox modules
}
type Compiler interface { Compile(ctx context.Context, doc *config.Document, cc CompileOptions) (Pipeline, error) }
type CompileOptions struct { Sandbox sandbox.Runtime; Library config.LibraryResolver; Codecs codec.FactoryManager;
                             Helpers ctxhelpers.Set; Clock ctxhelpers.Clock; RouteMeta RouteMeta }
type CompileError struct { Issues []config.Issue }          // mapped 1:1 into validation results

// codec
type Codec interface { Format() FormatType; Decode(ctx, raw []byte) (any, error); Encode(ctx, v any) ([]byte, error) }
type CodecFactory interface { Format() FormatType; New(spec config.Format) (Codec, error) }
type FactoryManager struct { factories map[FormatType]CodecFactory }   // registered at bootstrap
type SchemaRegistry interface { SchemaByID(ctx, id int32) (Schema, error); Latest(ctx, subject string) (Schema, error) }

// filter
type Node interface { ID() string; Eval(ec *EvalContext) (bool, error) }
type NodeCompiler interface { Type() NodeType; Compile(c *NodeCompileCtx, raw config.FilterNode) (Node, error) }
type Operator interface { Op() OpName; Accepts(t FieldType) bool; Prepare(arg OperandSpec) (Prepared, error) }
type Prepared interface { Match(left any, cz *Coercer) (bool, *CoercionWarning) }
type OperatorManager struct { ops map[OpName]Operator }

// step
type Step interface { ID() string; Kind() StepKind; Apply(sc *StepContext, in Event) ([]Event, error) }
type StepCompiler interface { Kind() StepKind; Compile(c *StepCompileCtx, raw config.Step) (Step, error) }
type StepCompilerManager struct { compilers map[StepKind]StepCompiler }

// formula
type Function interface { Name() FuncName; Arity() Arity; Call(args []any, h ctxhelpers.Set) (any, error) }
type FunctionCatalog struct { funcs map[FuncName]Function }

// trace
type Tracer interface { Enabled() bool; Node(branch BranchID, nodeID string, ok bool, w *CoercionWarning)
                        Step(branch BranchID, stepID string, before, after []Event, d time.Duration, err error)
                        Log(stepID, line string); Finish(status EventStatus) }
```

Selection is always by registry lookup keyed on the enum value from the document (Rule 15);
unknown keys produce `SchemaInvalid`/`CompileError`, never a panic or a type switch.

### Shared ctx helpers (`pkg/engine/ctxhelpers`)
`Set{Mask Masker; Hash Hasher; Clock Clock; Date DateFormatter; Lookup LookupStore; Log LogSink}`.
Mask styles enum: `full`, `last4`, `email`. Hasher = SHA-256(salt‖value), salt from a `SaltProvider`
port (platform-core secrets). `Clock.Now()` is frozen at event start; preview/tests inject `FixedClock`.
js-sandbox binds this exact `Set` as host functions, guaranteeing recipe ≡ JS behaviour.

### Coercion table (fixed, documented in code and in `engineCatalog.json`)
| Left value | Right operand | Rule | Warning |
| --- | --- | --- | --- |
| number | numeric string | parse right as float64 | no |
| numeric string | number | parse left as float64 | yes (`coerced_string_to_number`) |
| bool / "true"/"false" | bool | parse string | yes |
| RFC 3339 string / epoch ms | date op | parse to time | no |
| missing | any except exists/not_exists/is_empty | false | no |
| null | is_empty | true | no |

## 4. Data model touchpoints
- `route_versions.config` (JSONB) stores the canonical document; `config_hash` = `Canonicalizer.Hash`.
- `test_cases.input_event` / `expected_output` use the engine `Event` JSON shape (`key,value,headers,…`).
- `transform_versions.steps` uses the same `steps[]` schema fragment (`$defs/steps`).
- `lookup_table_versions.rows` = `[{key, value}]` (`$defs/lookupRows`).
- **No migration required** for this folder; 0001_init already has the columns.

## 5. API endpoints
This folder owns **no HTTP endpoints**. It owns the wire shapes other folders return:

Validation result (inside `POST /routes/{id}/validate`, owned by `routes`):
```json
{ "valid": false,
  "issues": [ { "code": "operator_type_mismatch", "severity": "warning",
                "path": "/branches/0/filter/children/1/op", "message": "gt on string field value.amount" } ] }
```
Deploy failure maps the first error issue to RFC 7807 `path` (platform-core problem.Details).

Trace (inside `POST /preview/run`, owned by `live-preview`), per event:
```json
{ "offset": 42, "status": "filtered",
  "branches": [ { "id": "main",
     "filter": [ { "node": "/children/0", "result": true }, { "node": "/children/1", "result": false,
                   "warning": "coerced_string_to_number" } ],
     "steps": [ { "id": "s1", "kind": "rename", "patch": [ {"op":"move","from":"/value/cust_id","path":"/value/customerId"} ],
                  "durationUs": 3, "logs": [], "error": null } ],
     "outputs": 1 } ],
  "error": null }
```
`EventStatus` enum: `passed`, `filtered`, `error`, `dropped`.

## 6. Flows

### 6.1 Validate (Save draft / Deploy)
```
HTTP POST /routes/{id}/validate | PATCH draft | POST deploy   (routes handler)
└─ routes ValidationManager calls ValidatorManager.Validate(stage, teamID, raw)   (doc-only part)
   └─ Document.Parse(raw)
      → JSON syntax error → [SchemaInvalid at "/"]; stop
   └─ for each validator in profile[stage] (ordered, configured)
      └─ SchemaValidator → violations → SchemaInvalid issues; schema errors short-circuit later validators
      └─ DeliveryModeValidator → exactly_once ∧ src.conn ≠ dst.conn → ExactlyOnceCrossCluster (error)
      └─ CompileValidator → engine.Compile(doc, dry-run options)
         → CompileError → PathInvalid / RegexInvalid / FormulaInvalid / JSCompile (error)
         → library ref missing (P3) → LibraryRefMissing (error)
         → ok → pipeline.Close()
   └─ Issues.Sort(by path) → returned to routes, merged with its external-data issues
      (ConnectionNotFound/Forbidden, TopicMissing, FieldUnknown, OperatorTypeMismatch,
       TestCaseFailed, LibraryUpdateAvailable — codes from this enum, rules in internal/routes/validation)
      → Save draft → always saved, issues listed
      → any error ∧ stage=deploy → routes rejects deploy (422 problem, JSON Pointer `path` of first error)
```
---
### 6.2 Compile (once per route version)
```
Worker loads version | preview run | CompileValidator
└─ PipelineCompiler.Compile(doc, opts)
   └─ Upgrader.ForVersion(doc.apiVersion) → unknown → CompileError(SchemaInvalid)
   └─ source codecs → FactoryManager.New(source.format / keyFormat)
      → unknown format → CompileError
   └─ for each branch
      └─ filter → NodeCompilerManager.Compile(ast)
         └─ cond → FieldPath.Parse(field) → invalid → PathInvalid at node pointer
         └─ cond → OperatorManager.Get(op).Prepare(operand) → regex/date parse fails → RegexInvalid
         └─ expr → sandbox.Runtime.Compile(wrapExpr(code), limits) → syntax error → JSCompile
      └─ steps → for each enabled step
         └─ kind=library → LibraryResolver.Transform(ref, ver) → splice steps inline (P3)
         └─ kind=lookup ∧ ref → LibraryResolver.LookupTable → load map (P3); inline rows (P2)
         └─ StepCompilerManager.Get(kind).Compile(step)
            → formula parse error → FormulaInvalid
            → js → sandbox.Runtime.Compile(code) → JSCompile
      └─ destination codec → FactoryManager.New(dest.format | same_as_source)
   └─ issues collected → any → CompileError{Issues}; release created modules
   └─ none → compiledPipeline{codecs, branches, limits, helpers}
```
---
### 6.3 Process one record (worker or preview)
```
Runner/preview receives RawRecord
└─ Pipeline.Decode(raw)
   → codec error → ProcessError{ErrDecode} → caller applies errorPolicy (route-runtime)
   → ok → Event; Clock frozen for this event; tracer.Enabled() decides snapshots
└─ Pipeline.Process(event, tracer)
   └─ for each branch (single branch "main" in MVP)
      └─ branch input = copy-on-write clone of event (only when >1 branch)
      └─ filter.Eval → false → tracer.Node(...) → branch yields 0 outputs (status filtered)
      └─ filter.Eval → error (expr sandbox) → ProcessError{ErrStep, step="filter"}
      └─ true → step chain (6.5)
         → outputs ≤ limits.maxOutputsPerEvent → Out{event, branch}
         → over limit → ProcessError{ErrLimit}
   └─ status = passed (≥1 out) | filtered (all branches filtered) | dropped (steps returned []) | error
   └─ tracer.Finish(status)
└─ for each Out → Pipeline.Encode(out)
   → encode/schema failure or > maxOutputBytes → ProcessError{ErrOutputInvalid} → DLQ by caller
   → ok → RawRecord → produced by runtime
```
---
### 6.4 Filter evaluation
```
filter.Eval(ec)
└─ and → children in order → first false → false (rest not evaluated, not traced)
└─ or  → children in order → first true → true
└─ not → !child
└─ cond → Accessor.Get(event)
   └─ Prepared.Match(left, Coercer)
      → coerced → result + CoercionWarning → tracer.Node(id, result, warning)
      → incomparable types → false + warning (never an error)
└─ expr → sandbox Module.Call({event}) → boolean
   → non-boolean → truthiness per JS + warning
   → timeout / exception → error → ErrStep / ErrStepTimeout
```
---
### 6.5 Step chain
```
StepChain.Run(sc, event)
└─ current = [event]
└─ for each step (disabled steps removed at compile)
   └─ for each ev in current → step.Apply(sc, ev)
      → [] → ev dropped (drop_if, js returns null)
      → [ev'] → replaced
      → [a,b,…] → split (split_array, js returns array)
      → error → ProcessError{ErrStep, stepID, path}; stop chain for this event
   └─ tracer.Enabled() → PatchBuilder.Diff(before, after) → tracer.Step(id, patch, duration)
└─ return current
```
---
### 6.6 Avro/Protobuf decode with schema registry (P2)
```
Codec.Decode(raw)
└─ len<5 ∨ raw[0]≠0x00 → ErrDecode("not schema-registry framed")
└─ schemaID = BigEndian(raw[1:5])
   └─ SchemaCache.Get(id) → hit → deserializer
   └─ miss → SchemaRegistry.SchemaByID(id) [network, per-connection client]
      → 404 → ErrDecode("unknown schema id") (sent to DLQ)
      → transient (timeout, 5xx) → ProcessError{ErrDecodeTransient} → runtime backs off, no DLQ, no commit
      → ok → SchemaCache.Put(id) (LRU, no TTL: IDs are immutable)
└─ deserialize → map[string]any (Avro: logical types → time/decimal string)
```
---
### 6.7 Type and catalogue generation (build time)
```
Developer runs `make gen` (also CI `make check-gen`)
└─ go-jsonschema schema/route.v1.schema.json → pkg/config/routeconfig_gen.go
└─ npm --prefix web run gen:types → web/src/api/generated/routeConfig.ts
└─ go run ./cmd/gen-catalog → web/src/api/generated/engineCatalog.json
   (operators by FieldType, coercion table, recipe kinds + groups, formula functions + signatures)
└─ check-gen → regenerate into scratch dir, diff → mismatch → CI fails
```
---
### 6.8 routerctl run (milestone 1 CLI)
```
CLI `routerctl run --config route.json --events in.jsonl [--trace] [--now 2026-01-01T00:00:00Z]`
└─ ValidatorManager(profile=offline: schema, delivery, compile) → errors → print issues, exit 2
└─ Compile with local sandbox, FixedClock(--now), in-memory library (--library dir)
└─ for each line → RawRecord (value = line bytes) → Decode → Process → Encode
   → output → stdout JSONL {branch, key, value, headers}
   → ProcessError → stderr JSONL {offset, type, step, message}; continue
   → --trace → stderr trace JSON per event
```

## 7. Error cases
| Case | Where | Result |
| --- | --- | --- |
| Unknown `kind` / `type` / `op` | schema / compile | `SchemaInvalid` with pointer |
| Field path syntax invalid (`value..a`, `items[x]`) | compile | `PathInvalid` |
| Path root not in {key,value,headers,topic,partition,offset,timestamp} | compile | `PathInvalid` |
| Regex fails to compile / RE2 unsupported syntax | compile | `RegexInvalid` |
| Formula parse error / unknown function / wrong arity | compile | `FormulaInvalid` |
| Missing field in non-strict step | runtime | no-op (rename/remove/mask skip silently) |
| Missing field with `strict: true` | runtime | `ErrStep` |
| convert_type failure (`"abc"` → number) | runtime | `ErrStep` unless `onFailure: null\|keep` |
| Split fan-out > `maxOutputsPerEvent` | runtime | `ErrLimit` |
| JS timeout / OOM / exception | runtime (sandbox) | `ErrStepTimeout` / `ErrStep` |
| Output exceeds `maxOutputBytes` or fails dest schema | encode | `ErrOutputInvalid` |
| Library ref to deleted version | compile | `LibraryRefMissing` |

## 8. Testing (SysDesign §13)
- Unit, table-driven: every operator × coercion row, every recipe, every formula function, path parser, codecs.
- Golden: `pkg/engine/testdata/golden/*` run by `TestGolden` (`-update` flag rewrites expected).
- Fuzz: `FuzzFilterASTRoundTrip`, `FuzzRecipesNoPanic`, `FuzzJSONCodec`, `FuzzFieldPath`, `FuzzFormulaParser`.
- Benchmarks with thresholds: `BenchmarkFilterOnly`, `BenchmarkRecipeChain`, `BenchmarkMapper`; JS benchmark lives in js-sandbox.
- Schema conformance: every golden `route.json` validates against schema and round-trips through Go + TS types.
- Parity (preview vs worker) is owned by `live-preview`/`route-runtime`; this folder provides deterministic Clock + golden corpus.

## 9. Decisions
- Issue and problem `path` use JSON Pointer (`/branches/0/filter/children/1`) everywhere (platform-core agrees).
- Types are generated from the JSON Schema: go-jsonschema (Go) + json-schema-to-typescript (TS); CI fails on drift.
- Filter `expr` mode is MVP. The routes P2 "Code mode" reuses the `expr` node; there is no new AST node.
- The engine catalogue ships as static generated JSON (`engineCatalog.json`). There is no runtime endpoint.
- Only doc-only rules stay in `pkg/config`: schema, delivery mode, dry-run compile. External-data rules
  (connection access, destination topic, field types vs samples, test cases, library refs incl.
  `LibraryUpdateAvailable` warning) are in `internal/routes/validation`. They are owned by routes and
  share this folder's `IssueCode` enum.
- `errorPolicy.dlqConnectionId` is added to the route schema by this folder; when it is absent, the source connection is used.
- `ctx.now` is the processing time, frozen per event (`SystemClock`). Preview and tests inject `FixedClock`.
- Hash salt is per team (via `SaltProvider`, implemented by js-sandbox on platform-core secrets).

## 10. Open questions (remaining)
1. Mask `email` style when local part has 1 char (`a@x.com` → `*@x.com`?) — propose full mask of local part.

## 11. P2 additions (as implemented)

Sections 1–10 are unchanged; this section records the P2 shapes and decisions.

### 11.1 Schema additions (`route.v1.schema.json`, flat `Step` object, rules by `if kind`)
| Kind | Required members | Optional members | Notes |
| --- | --- | --- | --- |
| `flatten` | `field` | `separator` (default `.`), `to` | objects only; arrays stay values |
| `redact_pattern` | `field` + (`preset` \| `pattern`) | `replacement` (default `[REDACTED]`), `to` | presets `credit_card`, `phone`, `email`, `iban` (RE2) |
| `format_date` | `field`, `layout` | `timezone` (IANA, default UTC), `to` | input RFC 3339 or epoch ms; tz checked at compile |
| `round` | `field`, `decimals` (0–15) | `to` | half away from zero; numeric text accepted |
| `string_case` | `field`, `case` | `to` | `upper`, `lower`, `title`, `trim`; non-text unchanged |
| `lookup` | `field` + `rows` | `to`, `defaultValue` | `ref`+`version` (named tables) is P3 → `library_ref_missing` today |
| `if_else` | `condition`, `thenSteps` | `elseSteps` | nested steps reuse `$defs/steps`; ids `<id>/thenSteps/<child>` |
| `split_array` | `field` | `to` | one event per element; `[]` drops; non-array → step error `type_mismatch` |
| `wrap` / `unwrap` | `field` (key/value member path) | — | wrap nests the whole root; unwrap lifts the member |
| `header_add` | `header`, `source` | — | set semantics (replaces same-name headers) |
| `header_remove` | `header` | — | strict → error when absent |
| `header_rename` | `header`, `newHeader` | — | |
| `header_copy_to_payload` / `_from_payload` | `header`, `field` | — | |
| `map` | `target` | `start` (`copy_input` default \| `empty`) | target tree = `$defs/mapField` |

`mapField = {source?: ValueSource, remove?: bool, fields?: {name: mapField}}`; the root addresses `value`.
`branches` now allows 1–16 items; branch ids must be unique (compile issue `schema_invalid` at `/branches/i/id`).
TS generation passes `--maxItems -1` so `branches` is `[Branch, ...Branch[]]`, not a tuple union.

### 11.2 Formula language (`pkg/engine/formula`)
Grammar: literals (int → int64, decimal → float64, `"..."`/`'...'` with `\n \t \r \\ \" \'`, `true false null`),
field paths (`value.a[0].b`, `headers.x`, `topic`), calls `fn(args)`, unary `- !`, binary by precedence
`|| < && < == != < < <= > >= < + - < * / %`. Limits: 4096 bytes, nesting depth 64.
Missing fields read as `null`. Arithmetic keeps int64 when both operands are integers (except `/`).
Functions are a registry (`FunctionCatalog`) with lazy `Args`, so `if()` and `coalesce()` evaluate only what they need.
`lookup(table, key, default?)` reads the route's inline tables (table name = lookup step id) first, then the injected
`ctxhelpers.LookupStore` (`LayeredLookupStore`).
```
Compile: ValueSource{from: formula, expr} | mapper leaf
└─ Lexer.Tokens(expr) → bad char / unterminated string → FormulaInvalid at …/expr (byte position in message)
└─ parser.expr (precedence climbing)
   → unknown function / wrong arity / bad path → FormulaInvalid
   → ok → Program (AST of literal | fieldRef | call | binary | unary)
---
Runtime: step resolves the value source
└─ Program.Eval(EvalContext{event, frozen now, helpers})
   → error (division by zero, not a number, bad date) → StepError formula_failed → ProcessError{ErrStep}
   → ok → value written by the step
```

### 11.3 Visual Mapper (`step.MapperCompiler`)
```
Compile map step
└─ MapTree.walk(target, value root) — children in sorted key order
   └─ source → ValueSourceManager.Compile → assignment(path, resolver)
   └─ remove: true → removal(path)
---
Apply(event)
└─ resolve every assignment against the unmodified input (missing ∧ strict → field_missing)
└─ value = start(copy_input → input value | empty → {})
└─ apply removals → apply assignments (missing leaves skipped)
```

### 11.4 Branches (PE-70)
Every branch except the last gets a deep `Clone` of the decoded event, the last one takes the original
(copy-on-write: one copy fewer per event). Outputs carry `Out.Branch`; the event status is
`passed` (any output) > `dropped` > `filtered`; any branch error fails the event (`ProcessError.Branch` set).

### 11.5 Schema registry codecs (PE-71..73)
- `codec.SchemaRegistry` port (`SchemaByID`, `Latest`, `Version`) + `RegistryProvider` (per connection, injected as
  `CompileOptions.SchemaRegistries`). `HTTPRegistryProvider` builds Confluent REST clients from a
  `RegistryConfigSource` port (to be implemented by connections#CN-102); `CachedRegistry` caches by id and
  subject version without TTL and `Latest` for 1 min. Compile never touches the network (`ConnectionRegistry`
  resolves lazily per call). No provider → records fail with `no schema registry configured` (decode error).
- Errors: 404 → `ErrSchemaNotFound` (decode → DLQ); network / 5xx / 429 → `ErrTransient` → `decode_transient`.
- Destinations encode with the latest schema of `format.subject`, default `<topic>-value` (key `<topic>-key`).
- Avro (hamba/avro v2): schema-guided value mapping both ways; logical types → RFC 3339 text (timestamps),
  `YYYY-MM-DD` (date), decimal text with schema scale, fixed → bytes; unions unwrap on decode and pick the first
  fitting member on encode. Reader limits: 16 MiB bytes/strings, 1 Mi array elements.
- Protobuf (protocompile + dynamicpb): references become imports, message indexes per Confluent framing,
  maps use proto field names, scalars include defaults, enums by name, `google.protobuf.Timestamp` ↔ RFC 3339;
  encode picks `format.messageType` (full or simple name) or the first message. Runtime panics on hostile input
  are recovered into `ErrMalformed`.
```
Codec.Encode(value)                                   (avro | protobuf destination)
└─ SubjectResolver.Latest(subject) [network, cached 1 min]
   → empty subject → ErrSubjectRequired → output_invalid
   → transient → ErrTransient (output_invalid today; route-runtime decides retries)
└─ schemas.ByID(id) → parsed schema LRU
└─ values.ToAvro / ToProto → mismatch → ErrUnencodable → output_invalid → DLQ by caller
└─ Framing.Write(id, [message indexes] + body)
```

### 11.6 Document formats (PE-81)
`config.DocumentFormatManager` with `json` and `yaml`. JSON → YAML streams JSON tokens into a `yaml.Node` tree, so
member order and number text survive; strings that would read as other YAML types are quoted (`!!str`).
YAML → JSON accepts exactly one document, resolves aliases (depth ≤ 64), rejects merge keys and non-JSON values
(`.inf`), and copies JSON-compatible number text verbatim. All golden routes round-trip byte-identically.

### 11.7 Catalogue v2 (PE-82)
`engineCatalog.json` adds `formulaFunctions` (name, `call` form, arity, params, returns, category, description,
example), `formulaOperators`, `redactPresets`, `stringCases`, `mapStarts`, `sourceFormats`, the `structure` and
`kafka` recipe groups and the `formula` value source. `web/src/api/generated/index.ts` exports `formulaFunctions`.

## 12. P3 additions (as implemented)

Sections 1–11 are unchanged; this section records the P3 shapes and decisions.

### 12.1 Library steps (PE-100)
- `{kind: library, ref, version}` expands the pinned transform at compile time. The expanded steps are
  **spliced** into the parent chain (`step.Splicer`): each one is traced, timed and reported on its own.
- Trace / error step ids: `<stepId>/<ref>@<version>/<childStepId>` (deviation from the plan's `<ref>@v/<stepId>`:
  the library step id is kept as prefix so two library steps pinning the same transform stay distinct and
  Studio can attribute trace steps to the route step; FE helper `LibraryStepIds`). Runtime `ProcessError.Path`
  is the library step pointer, because the expanded steps are not part of the route document.
- Compile issues inside the transform are re-reported at the library step pointer, with the in-library
  pointer in the message (`library transform clean@2 step /1/from: …`), keeping their own code.
- One `LibraryCache` per compile wraps `CompileOptions.Library`: each pin is loaded once per compile.
- Nested library steps are allowed up to `LibraryMaxDepth` (4); a ref that includes itself is refused.
- Library steps inside `if_else` run their expanded chain as one step (nested chains are not traced).
```
Compile step {kind: library, ref, version}
└─ ref/version absent → SchemaInvalid at …/ref
└─ CompileOptions.Library nil → LibraryRefMissing at …/ref ("no library is available")
└─ ref already on the expansion stack → LibraryRefMissing ("includes itself")
└─ stack depth ≥ 4 → LibraryRefMissing ("nests more than 4 library levels")
└─ LibraryCache.Transform(ref, version) [port → transform-studio StoreLibraryResolver, TS-208]
   → ErrLibraryRefMissing (unknown name, deleted version) → LibraryRefMissing at …/ref
   → other error (store down) → LibraryRefMissing at …/ref ("could not be loaded")
   → ok → compile steps in an isolated scope (ids prefixed `<stepId>/<ref>@<version>/`)
      → issues → re-reported at the library step pointer (own code, in-library pointer in message)
      → none → entries spliced into the parent chain (pointer = library step)
```

### 12.2 Library lookup tables (PE-101)
- `{kind: lookup, field, ref, version}` (no `rows`) loads `LibraryResolver.LookupTable(ref, version)` once per compile.
- The compiled table is registered in the route's `TableLookupStore` under the step id, `<ref>@<version>` and, as a
  lower-priority alias, the bare `<ref>`. A ref pinned at two versions keeps no bare alias (use `<ref>@<version>`).
- The same store backs the formula `lookup()` function and JS `ctx.lookup`: `compilectx.Scope.Script` binds
  `script.Source.Lookups` (the route's layered store) to every compiled module. The production sandbox adapter
  reads it in js-sandbox#JS-202; until then the QuickJS `ctx.lookup` sees only the injected base store.
- JS can only read library tables that the route pins through a lookup step (pinning is per route version).
```
Compile step {kind: lookup, ref, version}
└─ rows present → inline table (P2), registered under the step id
└─ ref/version → LibraryPins → LibraryCache.LookupTable
   → missing / error → LibraryRefMissing at …/ref
   → ok → Register(stepId), Register(ref@version), RegisterAlias(ref → ref@version)
---
Runtime lookup(table, key) | ctx.lookup(table, key) | lookup step
└─ LayeredLookupStore: route tables (name, then alias) → injected helpers.Lookup → miss → default / null
```

### 12.3 Replay and exactly-once (PE-103)
No engine change: a compiled pipeline is a pure function of (record, frozen clock, pinned library versions), with
no state between records. Replays reprocess offsets through the same compiled version and produce identical
output (`TestReplayIsDeterministic`); `decode_transient` stays the only "do not commit" signal for runners.

### 12.4 Catalogue v3 and release gate (PE-105, PE-112)
- `engineCatalog.json` v3 adds `library {pinnedKinds, versionSeparator, stepIdSeparator, stepIdForm,
  lookupTableNames, maxDepth}`; `web/src/api/generated` exports `libraryCatalog` and `LibraryStepIds`.
  The schema documents `ref`/`version`; regenerated Go/TS types (shape unchanged).
- `make engine-gate`: `go test` of pkg/config, pkg/engine, routerctl (goldens included), every engine fuzz
  target for `ENGINE_FUZZTIME` (default 60 s), then `engine-bench-gate`. CI: `.github/workflows/engine-gate.yml`
  (also runs `make check-gen`).
- Goldens may carry a `library/` directory (`<ref>@<version>.json` transforms, `tables/<ref>@<version>.json` rows),
  loaded by `configtest.LibraryDir`, which `routerctl --library` uses too.
