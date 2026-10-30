# Pipeline Engine — Design (00001-fast-codec)

Incremental version on top of `00000-init`. **Unchanged:** sections 1–12 of `00000-init/02-design.md` (packages,
document, interfaces, flows 6.1–6.8, errors, testing, P2/P3 additions). `Pipeline.Decode / Process / Encode`
keep their tree semantics; this version adds a fused record path used by the worker and changes flow 6.3
("Process one record") for the worker only.

## 1. New and changed packages

| Package / file | Role |
|---|---|
| `pkg/engine/codec/fastjson` | Byte-level JSON: `Scanner` → `Tape` (flat entries with spans and flags), `Materializer` (tape → Go values = `TreeReader`), `CanonicalWriter` (tape → bytes = `TreeWriter(TreeReader(raw))`), `MemberSorter`, `ValueWriter` (Go tree → bytes = `TreeWriter`), `NumberText`, `Strings` |
| `pkg/engine/projection` | `Recorder` (compile-time access log), `FieldTree`, `Analyzer` + `Check`s → `Plan`, `Codec` / `Document` (projected decode, patch encode, passthrough) |
| `pkg/engine/codec_plan.go` | `CodecMode` (fast, tree), `CodecFallback` reasons, `CodecPlan`, `CodecStats`, `Encoded`, `RecordRunner` |
| `pkg/engine/options.go` | `CompileOptions.Codec` (empty = `DefaultCodecMode` = fast) |
| `pkg/engine/compile/records.go` | `RecordStrategy` (`TreeRecords`, `ProjectedRecords`), `compiledPipeline.Run / CodecPlan / CodecStats`, `ProjectedValue` |
| `pkg/engine/compile/branch.go` | `OutputShaper.EncodeWith(…, ValueEncoder)`; `Encode` = `EncodeWith(NullableEncoder)` |
| `pkg/engine/compilectx/scope.go` | `Scope.Access *projection.Recorder`; `Path` and `Script` record |
| `step.StepCompilerManager`, `step.ValueSourceManager`, `filter.NodeCompilerManager`, `step.StepCompileCtx.Formula` | record kind / type / source / formula fields (one line each) |
| `pkg/engine/formula/fields.go` | `Program.Fields()` — AST walk for field references |
| `internal/runtime/runner/record_processor.go` | uses `engine.RecordRunner.Run` when the pipeline implements it |
| `internal/runtime/reconcile/codec_report.go` | log + `router_route_codec_total{codec,fallback}` per compiled version |
| `internal/platform/config/engine_codec.go`, `cmd/worker/assembly.go` | `ROUTER_ENGINE_CODEC` → `EngineVersionCompiler.Codec` |

## 2. Key types

```go
type RecordRunner interface {                 // pkg/engine
    Run(ctx, RawRecord) ([]Encoded, error)    // = Decode + Process(no tracer) + Encode of every output
    CodecPlan() CodecPlan                     // {Mode, Fallback, Detail}
}
type Check interface { Verify(Inputs) (engine.CodecFallback, string, bool) }   // projection
type Analyzer struct { Checks []Check }      // DefaultAnalyzer: Mode, Format, Script, Formula,
                                             // AllowList(steps), AllowList(nodes), AllowList(sources), WholeValue
type FieldNode struct { Name string; Children map[string]*FieldNode; Full bool }
```

`compile.Registries.Codec` holds the analyser (empty = default); allow-lists are `projection.PathBoundedSteps`,
`PathBoundedNodes`, `PathBoundedSources`. A step kind is path-bounded when its effect is confined to the paths it
parses through `Scope.Path` (plus key / headers): every recipe except `keep_only`, `wrap`, `unwrap`, `map`, `js`.

## 3. Flows

### 3.1 Compile (changes flow 6.2: one step added at the end)
```
PipelineCompiler.Compile(doc, opts)                                   [user action: deploy / worker reconcile]
└─ newScope → Scope.Access = NewRecorder()
└─ codecs, filters, steps, destinations compile as before
   └─ Scope.Path(raw)                → Recorder.Path(p)               (every filter / step / key-field path)
   └─ StepCompilerManager.Compile    → Recorder.Step(kind)            (nested if_else + library steps included)
   └─ NodeCompilerManager.Compile    → Recorder.Node(type)
   └─ ValueSourceManager.Compile     → Recorder.Source(kind)
   └─ StepCompileCtx.Formula         → Recorder.Formula(src, Program.Fields())
   └─ Scope.Script (js step, expr)   → Recorder.Script(pointer)
└─ issues? → CompileError (unchanged)
└─ Analyzer.Plan(Inputs{opts.Codec, source format, destination formats, Recorder})
   └─ first failing Check            → CodecPlan{tree, reason, detail}  → TreeRecords
   └─ all pass                       → CodecPlan{fast} + FieldTree     → ProjectedRecords
```

### 3.2 Worker record (changes flow 6.3 for the worker)
```
RecordProcessor.Process(raw)                                          [event: polled record]
└─ pipeline implements RecordRunner → Run(raw)   (else Decode/Process/Encode as before)
   └─ TreeRecords   → Decode → Process(Noop) → Encode each output     (tree plan)
   └─ ProjectedRecords
      └─ Codec.Decode(raw.Value)
         └─ nil / invalid / over limits / root not object → tree run of the record, CodecStats.Fallbacks++
         └─ Scanner.Scan → tape; project(root, FieldTree) → partial map (only planned members)
      └─ key codec decode (error → tree run)
      └─ Process(event with partial map, Noop tracer)  (error → tree run: same error as tree codec)
      └─ per output: OutputShaper.EncodeWith(ProjectedValue{doc, branch state})
         └─ branch has no steps (ValueUntouched)
            └─ input canonical → output = input bytes (passthrough, no copy)
            └─ else            → CanonicalWriter(root)
         └─ branch has steps (ValueEdited) → patch(root)
            └─ result == input and input canonical → output = input bytes
         └─ encode error → tree run of the record
      └─ Document.Release (tape back to pool; outputs stay valid)
```

### 3.3 Patch encode of one object
```
patch(obj, node, cur map)
└─ queue original members of obj (Ref = child node when projected)
└─ queue members of cur (Pos = after originals)
└─ sort by name, merge runs: current member wins (inherits the original's tape index); original-only
   └─ projected (Ref set)       → dropped (a step removed it)
   └─ outside the projection    → CanonicalWriter (verbatim copy when its subtree is canonical)
└─ current member
   └─ partial node + original was object + cur is map → patch(recursive)
   └─ else                                            → ValueWriter (tree-writer bytes)
```

### 3.4 Field tree
A path ends in a **full** node (decoded with its whole subtree). An array index makes the member before it full
(arrays are decoded whole). Members on the way are **partial** (only listed children are decoded). A partial
node whose original value is not an object is decoded whole. The analyser rejects `value` and `value[i]`.

## 4. Byte-identity rules (why outputs match the tree codec)
| Tree codec behaviour | Fast path |
|---|---|
| keys sorted (Go string order), compact | `MemberSorter` (8-byte prefix + bytes compare); verified reuse of the previous order (O(n)) for arrays of same-shaped objects; `FlagSorted` skips sorting |
| duplicate names: last wins | sort by (name, position), keep last |
| strings: unescape (invalid UTF-8, lone surrogates → U+FFFD), re-quote minimal, no HTML escaping | plain valid UTF-8 copied; others `jsontext.AppendUnquote` + `AppendQuote` (same functions) |
| numbers: int64 if `ParseInt` ok, else float64 (`jsontext.AppendFloat`), else raw `json.Number` | `NumberText.Append` (same order); canonical shortcut only for ints ≤ 18 digits and `d.d` decimals ≤ 15 significant digits, no trailing 0, ≥ 1e-6 (property-tested) |
| NaN / ±Inf, unknown Go types | delegated to `codec.TreeWriter` for that value |
| errors | any fast-path error re-runs the record on the tree codec, so errors are the tree codec's |

`FlagCanonical` (raw bytes = tree output) is conservative: false only costs a re-print.

## 5. Limits and assumptions
- Scanner limits: depth 512, 2 GiB; deeper / larger values take the tree path (tree max depth is 10000).
- Formula functions must not read `EvalContext.Event` directly (all current functions read arguments only); a
  function that does must make `Program.Fields` return false.
- New step kinds, filter node types and value sources run on the tree codec until added to the allow-lists.
- Passthrough output shares the input byte slice; record values are never mutated in place downstream.

## 6. Testing
| Test | What |
|---|---|
| `fastjson` `TestCanonicalParity*`, `TestNumberCanonicalShortcut`, `FuzzCanonicalWriter` | scanner acceptance = tree reader; materialized values, canonical bytes and value-writer bytes = tree reader/writer; 20k random docs + fixed edge cases |
| `pkg/engine` `TestFastCodecGoldenParity` | every engine + preview golden route, every input line + 150 mutations each, fast vs tree records and errors |
| `pkg/engine` `TestFastCodecRouteParity`, `FuzzFastVsTree` | synthetic routes over every allow-listed kind, nested / partial / indexed paths, 3 branches, key-from-field |
| `compile` `TestProjectedRecords*` | fast path finishes without fallback on bench shapes; passthrough shares input; unsorted input canonicalised |

## 7. Results (S1, one pinned core, Xeon Gold 6148, GOMAXPROCS=1, 3 s × 3)
Source `test/stress/results/s1-fast-codec.json` (`test/stress/s1-fast-codec.sh`, raw + profiles in
`results/s1-fast-codec/{tree,fast}/`). The host was shared with other jobs (load 9–15 on 14 cores): treat
single cells as ±15–30 %; CPU-time metrics (`events/cpu-s`) matched wall clock, so cores were not stolen.

| events/s per core (engine) | 1 KB tree → fast | 10 KB | 100 KB | allocs/op 1 KB |
|---|---|---|---|---|
| filter-only | 16.0k → **111.7k** (7.0×) | 1.95k → 19.5k (10×) | 220 → 2.1k (9.6×) | 261 → 16 |
| filter-1 / 10 / 50 | 14.5k / 15.3k / 14.3k → 76k / 69k / 58k | ~2.0k → 10.3k / 10.4k / 8.5k | ~215 → 1.1k / 1.1k / 0.9k | 262 → 14–17 |
| steps-1 / 5 / 20 | 14.8k / 13.8k / 10.8k → 64k / 46k / 22k | ~1.9k → 9.7k / 9.5k / 7.2k | ~213 → 1.1k / 1.1k / 1.0k | 265–333 → 14–85 |
| recipe-5 | 14.4k → **44.1k** (3.1×) | 2.0k → 12.9k (6.5×) | 213 → 1.7k (7.8×) | 277 → 42 |
| mapper (tree fallback) | 12.4k → 11.7k | 1.2k → 1.5k | 150 → 168 | unchanged |
| js, steps-5 + js (fallback) | 745 / 803 → 834 / 775 | ~135 | 15 | unchanged |

Runner (`RecordProcessor`, worker CPU path): filter-only 16.0k → 72.4k (1 KB), 2.2k → 10.4k (10 KB),
232 → 1.1k (100 KB); recipe-5 14.8k → 39.9k, 2.0k → 9.4k, 237 → 1.1k; mapper / js unchanged (fallback).
Inputs already in output form (`BenchmarkS1EngineCanonicalInput`, ad hoc A/B): filter-only 1 KB 16.7k → 109.6k,
10 KB 2.0k → 18.9k, 100 KB 186 → 2.3k (passthrough).

Remaining cost at 1 KB filter-only (fast): scan ~34 %, canonical rewrite of the unsorted input ~28 %,
malloc/GC ~22 %, filter + branch run ~13 %. At recipe-5 1 KB the five recipe steps themselves (~8 µs, also on
the tree path) now dominate. The NFR of 100k/core is met for filter routes; recipe chains are bound by step cost.

## 8. Decisions
- D-FC-1 Fused `RecordRunner.Run` instead of a partial `Event.Value` from `Decode`: the projected map never leaves
  the engine, so preview, tracing, tests and every `Decode` caller keep full trees.
- D-FC-2 Byte identity over field order (sorted keys), since the tree codec defines today's output.
- D-FC-3 Re-run on the tree codec for any fast-path error: identical errors by construction; costs only on
  failing records.
- D-FC-4 Allow-lists live in `projection`, not on step compilers, so step files stay untouched and unknown
  kinds are safe by default.
