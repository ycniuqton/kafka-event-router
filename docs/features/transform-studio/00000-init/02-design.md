# Transform Studio — Design (00000-init)

Scope: the editing experience for the `transform` part of a route branch and its supporting
control-plane services. Step semantics live in `docs/systems/pipeline-engine`. The JS runtime lives in
`docs/systems/js-sandbox`. Sample sessions and `/preview/run` live in `docs/features/live-preview`.
This folder does not repeat those designs. It only lists the contracts it consumes.

## 1. Component map

```
web/src/pages/builder/studio  ──(debounced)──▶ live-preview PreviewService.run  ──▶ cmd/preview (pkg/engine)
        │                                                                     ▲
        ├──▶ StudioService ──▶ cmd/api /api/v1/assist/*, /preview/infer-mapping ─ internal/assist
        ├──▶ TestCasesService ──▶ cmd/api /api/v1/{routes|transforms}/{id}/test-cases ─ internal/library
        └──▶ LibraryService ──▶ cmd/api /api/v1/transforms, /lookup-tables ─ internal/library
                                                     │
                                  pkg/engine.Compiler (test runs, publish compile)
                                  pkg/config validators (proposal + publish validation)
                                  internal/store (Postgres)
```

The API runs test cases and publish-compiles in-process with `pkg/engine`, so this needs no
Kafka. Interactive preview of samples stays in the preview service.

## 2. Backend — `internal/library`

### 2.1 Domain types and enums (`internal/library/domain.go`)
```go
type TransformID string; type LookupTableID string; type TestCaseID string

type Transform struct { ID TransformID; TeamID string; Name, Description string
    LatestVersion int; ArchivedAt *time.Time; CreatedAt, UpdatedAt time.Time; ETag string }
type TransformVersion struct { ID string; TransformID TransformID; VersionNo int
    Steps json.RawMessage; ConfigHash, ChangeNote string; CreatedBy string; CreatedAt time.Time }
type LookupTable struct { ID LookupTableID; TeamID, Name, Description string; KeyType ValueType
    LatestVersion int; ArchivedAt *time.Time; ETag string }
type LookupTableVersion struct { ID string; TableID LookupTableID; VersionNo int
    Rows []LookupRow; RowCount int; ChangeNote, CreatedBy string; CreatedAt time.Time }
type LookupRow struct { Key string; Value any }
type TestCase struct { ID TestCaseID; Owner TestCaseOwner; BranchID, Name string
    InputEvent json.RawMessage; ExpectedOutcome Outcome; ExpectedOutput json.RawMessage
    CreatedBy string; CreatedAt, UpdatedAt time.Time }
type TestCaseOwner struct { Kind OwnerKind; ID string }        // route | transform

type OwnerKind string    // OwnerRoute="route", OwnerTransform="transform"
type Outcome string      // OutcomePassed, OutcomeFiltered, OutcomeDropped, OutcomeError (mirrors live-preview statuses)
type CaseResult string   // ResultPass, ResultFail, ResultError
type ValueType string    // ValueString, ValueNumber, ValueBoolean
type ImportFormat string // ImportCSV="text/csv", ImportJSON="application/json"

type Limits struct{ MaxLookupRows, MaxLookupBytes, MaxTestCasesPerOwner, MaxCaseEventBytes int }
var DefaultLimits = Limits{50_000, 5 << 20, 200, 256 << 10}
```

### 2.2 Repositories (`internal/store/library_repo.go`, built on `store.Repository[T,ID]`)
- `TransformRepository`: generic CRUD plus `GetByName(team, name)`, `AppendVersion(ctx, v) (TransformVersion, error)`. `AppendVersion` uses `SELECT … FOR UPDATE` on the parent, then sets `version_no = latest+1`.
- `LookupTableRepository`: same shape as `TransformRepository`.
- `TestCaseRepository`: `ListByOwner(owner)`, `CountByOwner(owner)`.
- `UsageRepository`: `TransformUsages(name) []Usage`. It is a jsonb_path query over `routes.draft_config` and active `route_versions.config` for `$.branches[*].transform.steps[*] ? (@.kind=="library" && @.ref==$name)`. `LookupUsages` does the same for lookup recipes and is best effort for `ctx.lookup` literals.

### 2.3 Services (business logic talks only to these)
| Class | Responsibility |
| --- | --- |
| `TransformLibrary` | create, update metadata (If-Match), archive (refused if in use), publish version (validate → compile → hash → append), list versions, usages, latest-version lookup for badges |
| `LookupTableLibrary` | create, metadata, publish version from rows or import, get rows (paged), usages |
| `RowImporterManager` | picks a `RowImporter` by `ImportFormat` from a config registry; enforces `Limits` |
| `RowImporter` (interface) | `Format() ImportFormat; Import(r io.Reader, keyType ValueType) ([]LookupRow, []ImportIssue, error)`. Impls: `CSVRowImporter`, `JSONRowImporter` |
| `TestCaseService` | CRUD of cases for a route or transform; `Run(ctx, owner, draft) RunReport` |
| `TestCaseRunner` | compiles the draft branch config via `engine.Compiler`, processes each input, classifies outcome |
| `OutcomeComparator` (interface) + `ComparatorManager` | compare expected vs actual (`ExactJSONComparator` default; future: ignore-paths). Returns RFC6902 diff |
| `TestCaseService.RunForRoute` | port consumed by routes' `TestCaseValidator` (`internal/routes/validation`, external-data rule): error on Deploy, warning on Save draft / Rollback. No validator class here |
| `StoreLibraryResolver` | implements pipeline-engine's `engine.LibraryResolver` and `engine.LookupResolver` (load pinned versions for compile) |

### 2.4 HTTP handlers (class-based, `internal/library/http/`)
`TransformsHandler`, `LookupTablesHandler`, `TestCasesHandler`. Each implements platform-core's
`api.Module` (`Mount(r chi.Router)`), uses RFC7807 problems with `path`, cursor pagination, and the `If-Match`
header on PATCH. Every mutating call goes through the audit middleware.

## 3. Backend — `internal/assist`

### 3.1 LLM provider family
```go
type ProviderKind string            // ProviderMock = "mock" (only one shipped)
type Prompt struct { Task AssistTask; Instruction string; FieldSchema json.RawMessage
    Samples []json.RawMessage; Steps json.RawMessage; StepSchema json.RawMessage }
type AssistTask string              // TaskProposeSteps, TaskExplain
type Completion struct { Text string; Steps json.RawMessage; Model string; Usage TokenUsage }

type LLMProvider interface {        // base contract; every provider implements it
    Kind() ProviderKind
    Complete(ctx context.Context, p Prompt) (Completion, error)
}
type ProviderManager struct{ providers map[ProviderKind]LLMProvider; active ProviderKind; limits AssistLimits }
func (m *ProviderManager) Complete(ctx context.Context, p Prompt) (Completion, error) // timeout, size caps, metrics
```
- `MockProvider`: deterministic. A `MockIntentParser` splits the instruction into clauses ("hide/mask X",
  "rename A to B", "remove X", "only keep …", "set X to V", "hash X"). It resolves field names
  against `FieldSchema` with fuzzy matching and emits recipe steps. For explain, it walks a JS AST summary
  (`MockExplainer` produces template sentences per detected pattern) or describes recipes.
- Selection: env `ROUTER_ASSIST_PROVIDER` (default `mock`). Unknown kind → startup error. The registry is filled in
  `cmd/api` wiring, so business code never names `MockProvider`.
- `AssistLimits{MaxInstructionChars:2000, MaxSamples:2, MaxSampleBytes:16<<10, Timeout:20s, MaxSteps:30}`.

### 3.2 Services
| Class | Responsibility |
| --- | --- |
| `StepProposer` | builds `Prompt` (masked samples via live-preview `SampleMasker`), calls `ProviderManager`, then runs `ProposalPolicy` and validates with `pkg/config` validators. Returns a proposal with issues |
| `ProposalPolicy` | prefers recipes: rejects or flags JS steps when a recipe equivalent exists, strips unknown kinds, caps the step count |
| `StepExplainer` | `ExplainStrategy` interface + `ExplainManager` keyed by step kind group: `RecipeExplainer` (deterministic text from step config), `JsExplainer` (via `ProviderManager`) |
| `MappingInferrer` | §7.6 infer mapping: flatten both trees → run `LeafMatcher` chain → `StepEmitter` → confidence |
| `LeafMatcher` (interface) + `MatcherManager` | ordered chain from config: `EqualValueMatcher` (1.0 / 0.8 if ambiguous), `NormalisedNameMatcher` (0.7), `TypeShapeMatcher` (0.4). First match wins per target leaf |
| `StepEmitter` | from matches emits `rename` (same parent), `move` (different parent), `set` constant (unmatched target with a literal), `remove` (unused input leaves) |
| `JsExporter` | P2 export-to-JS. `StepJsWriter` interface per step kind + `JsWriterManager` registry. Kinds that are not supported → 422 with `path` |

### 3.3 Handlers (`internal/assist/http/`)
`AssistHandler` (propose, explain, export-js) and `InferMappingHandler`. Rate limit per user
through platform-core's Redis limiter (`assist` bucket, 30/min).

## 4. API endpoints owned (all under `/api/v1`, JSON, RFC7807 errors)

| Method & path | Phase | Request → Response |
| --- | --- | --- |
| `GET /routes/{id}/test-cases` | P2 | → `{items:[TestCase]}` |
| `POST /routes/{id}/test-cases` | P2 | `{branchId,name,inputEvent,expectedOutcome,expectedOutput}` → `201 TestCase` |
| `PATCH /routes/{id}/test-cases/{caseId}` | P2 | If-Match; `{name?,expectedOutcome?,expectedOutput?}` → `TestCase` |
| `DELETE /routes/{id}/test-cases/{caseId}` | P2 | → 204 |
| `POST /routes/{id}/test-cases/run` | P2 | `{config: RouteConfig(draft)}` or `{steps, caseIds?}` → `{results:[{caseId,result,actualOutcome,actualOutput,diff:[JsonPatchOp],error?}], passed, failed}` |
| `…/transforms/{id}/test-cases[...]` | P3 | same shapes; `run` takes `{steps}` |
| `POST /preview/infer-mapping` | P3 | `{input: Event, desiredOutput: object}` → `{steps:[Step], mappings:[{target,source?,reason:"value"|"name"|"type"|"constant",confidence}], unmapped:[path]}` |
| `POST /assist/steps` | P3 | `{routeId, branchId, connectionId, instruction, sessionId, currentSteps}` → `{proposalId, steps:[Step], diff:{added,removed,changed}, issues:[Issue], provider}` |
| `POST /assist/explain` | P3 | `{step: Step}` or `{steps:[Step]}` → `{summary, bullets:[string], provider}` |
| `POST /assist/export-js` | P2 | `{steps:[Step]}` → `{step: {kind:"js",code}}` |
| `GET /transforms?cursor&limit&q&names=` | P3 | → `{items:[Transform+latestVersion+usageCount], next}` |
| `POST /transforms` | P3 | `{name,description,steps,changeNote}` → `201 {transform, version}` |
| `GET/PATCH/DELETE /transforms/{id}` | P3 | PATCH If-Match `{description}`; DELETE = archive, 409 if in use |
| `GET /transforms/{id}/versions`, `GET …/versions/{n}` | P3 | → versions with steps |
| `POST /transforms/{id}/versions` | P3 | `{steps,changeNote}` → `201 TransformVersion` (validate + compile; 422 with paths) |
| `GET /transforms/{id}/usages` | P3 | → `{items:[{routeId,routeName,version,inDraft}]}` |
| `GET/POST /lookup-tables`, `GET/PATCH/DELETE /lookup-tables/{id}` | P3 | mirror transforms; `keyType` on create |
| `POST /lookup-tables/{id}/versions` | P3 | `application/json {rows,changeNote}` or `text/csv` body (+`?changeNote`) → `201` or `422 {issues:[{line,message}]}` |
| `GET /lookup-tables/{id}/versions/{n}/rows?cursor&limit` | P3 | → paged rows |
| `GET /lookup-tables/{id}/usages` | P3 | → usages |

In the MVP the inline lookup recipe needs no endpoint: rows are kept in the step config.

## 5. Data model touchpoints (P2 migration `00NN_transform_studio_test_cases`, P3 migration `00NN_transform_studio_library`; numbers ≥ 0010, next free at implementation)
Existing tables from `0001_init`: `transforms`, `transform_versions`, `lookup_tables`,
`lookup_table_versions`, `test_cases`. Changes:
- `transforms`: add `description text NOT NULL DEFAULT ''`, `archived_at timestamptz`.
- `transform_versions`: add `config_hash text NOT NULL`, `change_note text`.
- `lookup_tables`: add `description text NOT NULL DEFAULT ''`, `key_type text NOT NULL DEFAULT 'string'` (CHECK in enum list), `archived_at`.
- `lookup_table_versions`: add `row_count integer NOT NULL`, `change_note text`.
- `test_cases`: add `branch_id text NOT NULL DEFAULT 'main'`, `expected_outcome text NOT NULL DEFAULT 'passed'` (CHECK), `created_by uuid REFERENCES users ON DELETE SET NULL`, `updated_at timestamptz NOT NULL DEFAULT now()`. Allow `expected_output` to be NULL when the outcome is filtered or dropped.
- Index (P3 migration, index only, no column change on routes tables): `CREATE INDEX route_versions_config_gin ON route_versions USING gin (config jsonb_path_ops)` for usage queries.

## 6. Frontend

### 6.1 Files
```
web/src/pages/builder/studio/
  TransformStudio.tsx(.module.css)      3-column shell; owns StudioController
  StudioController.ts                   class: selected step, sample index, output-at-step, diff mode; debounced preview
  input/InputPanel.tsx, JsonTree.tsx, TableView.tsx, EventStepper.tsx, ValueContextMenu.tsx
  steps/StepsPanel.tsx, StepCard.tsx, StepMenu.tsx, StepExample.tsx
  steps/recipes/RecipeFormRegistry.ts   StepKind → form component (config map, no switch)
  steps/recipes/*Form.tsx               one per recipe group (Rename, Mask, Lookup, IfElse, …)
  steps/js/JsStepEditor.tsx             lazy Monaco; markers from run errors
  steps/js/MonacoTypeDefBuilder.ts      FieldSchema → `declare` d.ts for event.value + ctx
  steps/js/SnippetCatalog.ts            domain constants of snippets
  steps/js/ConsoleTab.tsx
  output/OutputPanel.tsx, JsonDiffView.tsx, OutputMeta.tsx (key, headers, bytes)
  output/PatchReplayer.ts               rebuilds per-step snapshots from preview JSON patches
  mapper/VisualMapper.tsx, OutputTreeEditor.tsx, ConnectorLayer.tsx, ValueSourcePicker.tsx, FormulaBar.tsx   (P2)
  tests/TestCasesPanel.tsx, TestCaseBadge.tsx, TestCaseController.ts   (P2)
  magic/ByExampleDialog.tsx, AssistPrompt.tsx, ProposalDiff.tsx, ExplainPanel.tsx   (P3)
  library/LibraryStepCard.tsx, UpdateAvailableBadge.tsx   (P3)
web/src/pages/library/
  LibraryPage.tsx (tabs: Transforms | Lookup tables), TransformDetail.tsx, LookupTableDetail.tsx,
  LookupGridEditor.tsx, CsvImportDialog.tsx, VersionList.tsx, UsageList.tsx
web/src/domain/transform.ts            extend to generated step kinds (from pipeline-engine TS types)
web/src/domain/library.ts              Transform, TransformVersion, LookupTable, TestCase types
web/src/api/contracts.ts               StudioService, TestCasesService; LibraryService widened
web/src/api/mock/MockStudioService.ts, MockTestCasesService.ts, MockLibraryService.ts
web/src/api/http/*                     real /api/v1 clients (swap via registry config)
```

### 6.2 FE service contracts
```ts
abstract class StudioService { inferMapping(i: InferRequest): Promise<InferResult>;
  proposeSteps(r: AssistRequest): Promise<Proposal>; explain(steps: Step[]): Promise<Explanation>;
  exportJs(steps: Step[]): Promise<Step>; }
abstract class TestCasesService { list(o: Owner): Promise<TestCase[]>; create(o: Owner, c: NewTestCase): Promise<TestCase>;
  update(o: Owner, c: TestCase): Promise<TestCase>; remove(o: Owner, id: string): Promise<void>;
  run(o: Owner, draft: RouteConfig | Step[]): Promise<RunReport>; }
abstract class LibraryService { listTransforms(q): Page<Transform>; getTransform(id); createTransform(..);
  publishVersion(id, steps, note); versions(id); usages(id); latestVersions(names: string[]);
  listLookupTables(q); publishLookupRows(id, rows | File, note); rows(id, n, cursor); }
```
`RecipeFormRegistry`, `SnippetCatalog` and the step-menu groups are domain constant objects. New recipe
kinds are added by registering an entry, with no switch on kind.

## 7. Flows

```
User edits a step (field, reorder, toggle, add)
└─ StudioController.updateSteps(draft)  → step card marked dirty; route draft dirty (routes)
   └─ debounce 300 ms → PreviewService.run({config: draft, sessionId, trace: true})   [async]
      ├─ ok → PatchReplayer builds per-step snapshots → Output/Diff/Example/Console refresh
      │  └─ test cases loaded (P2) → TestCaseController.rerun(draft)   [async]
      ├─ compile error with path → StepCard shows red warning at path; output keeps last good
      └─ newer edit arrived → response discarded (request id older than latest)
```
---
```
User clicks "Run on sample" in a JS step
└─ PreviewService.run(draft, sessionId, trace) [async]
   ├─ all events ok → console shows ctx.log per event; badge "50/50 ok"
   └─ event k fails at step s → Monaco marker at line/col from sandbox error → "Event 7: <msg>"
      └─ user clicks "jump to event" → EventStepper.index = k; Output shows output at step s
```
---
```
User right-clicks a value in Input → picks action (Mask / Rename / Remove / Use as key)
└─ ValueContextMenu → ClickActionRegistry[action].buildStep(path, value)
   ├─ Rename → inline name prompt → rename step appended after selected step
   ├─ Mask → mask step with style guessed by value shape (email/last4/full)
   └─ Use as key → set_key step → preview re-run (see edit flow)
```
---
```
User pastes desired output (by example, P3)
└─ ByExampleDialog → POST /preview/infer-mapping {input: current event, desiredOutput}
   └─ InferMappingHandler → MappingInferrer.infer
      ├─ desiredOutput not a JSON object → 422 path "desiredOutput"
      └─ flatten trees → MatcherManager chain per target leaf → StepEmitter → steps + confidences
         └─ FE ProposalDiff (confidence < 0.6 highlighted amber) → user accepts → steps appended → preview
```
---
```
User submits an AI assist instruction (P3)
└─ POST /assist/steps → RBAC editor check → rate limit (429 if exceeded)
   └─ StepProposer.build prompt: field schema + ≤2 samples (masked if connection sensitive) + current steps
      └─ ProviderManager.Complete (timeout 20 s) → active provider (mock)
         ├─ provider error / timeout → 502 problem "assist unavailable"; UI keeps prompt text
         └─ completion → ProposalPolicy (prefer recipes, cap steps) → pkg/config validate
            ├─ blocking issues → returned with proposal, Accept disabled for invalid steps
            └─ valid → ProposalDiff shown → accept → steps replaced/appended; reject → no change
               └─ audit "assist.proposed" (instruction length, step count; no sample values)
```
---
```
User clicks "Explain" on a JS step (P3)
└─ POST /assist/explain {step}
   └─ ExplainManager picks strategy by step kind
      ├─ recipe kinds → RecipeExplainer → deterministic text
      └─ js → JsExplainer → ProviderManager.Complete(TaskExplain) → summary shown under step (not persisted)
```
---
```
User pins sample event as test case (P2)
└─ TestCasesPanel.pin(event, currentOutput, status) → POST /routes/{id}/test-cases
   ├─ count ≥ 200 or event > 256 KB → 422
   └─ saved → audit "test_case.created"
      └─ POST …/run {steps, caseIds:[id]}   [async]
         ├─ pass → badge list updated
         └─ differs (masked preview values) → PATCH If-Match with the actual result → badge list updated
```
---
```
Studio edit completes preview (P2)
└─ TestCaseController.rerun → POST /routes/{id}/test-cases/run {config: draft}   [async]
   └─ TestCaseRunner: engine.Compile(draft) → per case Process → ComparatorManager.compare
      ├─ compile error → all cases "error" with message
      ├─ outcome+output equal → pass badge
      └─ differs → fail badge + RFC6902 diff → shown in TestCasesPanel
```
---
```
Deploy requested (routes)
└─ routes TestCaseValidator (internal/routes/validation) → TestCaseService.RunForRoute(draft)   (runs in cmd/api)
   ├─ any fail ∧ deploy → Issue{Severity:error, Path:"/branches/{i}/transform"} → deploy blocked
   ├─ any fail ∧ draft / rollback → same issue as warning
   └─ all pass → continue deploy
```
---
```
User saves steps to the library (P3)
└─ POST /transforms {name, steps} or POST /transforms/{id}/versions
   └─ TransformLibrary.publish → RBAC editor on team → validate steps schema → engine compile (library refs, JS)
      ├─ invalid → 422 issues with paths
      ├─ name taken → 409
      └─ ok → hash steps → AppendVersion (tx, version_no+1) → audit "transform.version_published"
         └─ routes referencing older versions → GET /transforms?names= → "Update available" badge
            └─ user clicks Update → step.version = latest → preview + test cases re-run (no auto-change)
```
---
```
User uploads a CSV to a lookup table (P3)
└─ POST /lookup-tables/{id}/versions (text/csv)
   └─ RowImporterManager.forFormat(text/csv) → CSVRowImporter.Import(keyType)
      ├─ > 50k rows or > 5 MB → 413 problem
      ├─ bad lines / duplicate keys → 422 {issues:[{line,message}]}; nothing saved
      └─ ok → AppendVersion(rows,row_count) → audit → tables referencing via recipe show "Update available"
```
---
```
User chooses "Export to JS" on selected recipe steps (P2)
└─ POST /assist/export-js {steps}
   └─ JsExporter → JsWriterManager[kind].write per step → one js step source
      ├─ unsupported kind (e.g. library, map) → 422 path /steps/{i}
      └─ ok → FE replaces selection with js step → preview proves parity on samples
```
---
```
Studio opens or the sample session changes
└─ live-preview field schema (paths, types, top values)
   └─ MonacoTypeDefBuilder → d.ts for event.value + ctx → monaco.addExtraLib (replaces previous lib)
```

## 8. Error cases
| Case | Handling |
| --- | --- |
| Preview service down | Output panel shows an inline error with Retry. Steps stay editable. Test cases are not run |
| Draft step invalid (missing field) | Card shows a red warning from the validator `path`. Preview skips the step (engine error trace) |
| JS timeout / memory | Sandbox error per event. The card warns "Event k exceeded 50 ms". At runtime the event goes to the DLQ (route-runtime) |
| Library version deleted or archived while referenced | Archive returns 409 with usages. Compile reports the missing ref with its path |
| Concurrent metadata edit | 412 on If-Match mismatch. The UI reloads and re-applies the change |
| Assist proposes unknown/invalid steps | Policy strips them and lists issues. Nothing is applied |
| Assist instruction too long / rate limited | 422 path `/instruction` / 429 with Retry-After |
| Test case input no longer decodes (schema drift) | Case result `error` with message. It blocks deploy until updated or deleted |
| CSV encoding not UTF-8 | 422 line 0 "encoding" |
| Viewer opens Studio | Read-only. Mutating buttons are hidden. Samples are masked when the connection is sensitive (live-preview) |

## 9. Decisions
1. MVP uses inline lookup tables (rows in the recipe step). Named, versioned tables and `ctx.lookup` come in P3.
2. Click-to-transform is MVP.
3. Test cases run in `cmd/api` with the same `pkg/engine`, not in the preview service. The deploy/draft rule is a
   routes validator (`internal/routes/validation`) that calls `TestCaseService`.
4. The library is readable by all teams (open read). Writes need editor+ on the owning team.
5. Explain covers recipe chains (rule-based `RecipeExplainer`) and JS steps (`JsExplainer` via the LLM provider).
6. Below 1440 px, the Output column and the sample panel share the right zone as tabs.
7. AI uses a provider-agnostic `LLMProvider` interface. Only the mock provider ships.
8. The sample panel masks with live-preview's type-preserving masker. Viewers are always masked, other roles on sensitive connections.
9. Issue paths are JSON Pointers.
10. The Studio runs on live-preview's `services.preview.run` with the builder's samples as inline events and a
    stand-alone route document holding only the runnable steps (`studio/preview/StudioRunPlan`). Incomplete steps
    are skipped client-side; a step the preview rejects (422 path) is left out of the next attempt and flagged.
11. The recipe menu offers exactly the engine's recipe kinds (engineCatalog); kinds of pipeline-engine PE-76..78
    appear when the engine ships them.
12. The library MVP slice (list/create/publish versions for transforms and lookup tables) uses the 0001_init
    columns only; one `library.Service` serves both kinds through `ContentKind` + `KindManager`.
13. Test cases (P2) live in `internal/library/testcases` (+ `testcaseshttp`), migration `1080`. `POST …/run` takes
    `{config}` (full document, used by the routes validator) or `{steps, caseIds?}` (the Studio's steps merged into the
    route's stored draft, first branch). Each branch is compiled alone so the event status is that branch's.
14. A locked output is `{key, value, headers{}}` (array on fan-out, null unless passed). Pinning confirms the Studio's
    (possibly masked) expectation with a one-case run and locks the engine's actual result when they differ.
15. Callers without `sample.read_raw` on the route's team (Viewers) get test-case events, outputs and diff values masked
    (live-preview's type-preserving masker). Test runs use the team hash salt and the JS sandbox in test scope.
16. The Visual Mapper is one `map` step: entries are dotted output paths with a value source (field, fixed, formula,
    metadata) or a removal; "Paste an example" waits for P3 by-example. Formula live results come from the preview trace.

```
Studio edit (debounced 300 ms)
└─ StudioPreviewController.schedule(steps, samples)
   └─ StudioRunPlan: skip incomplete steps → POST /preview/run {events, config}   [async]
      ├─ newer edit → previous run aborted, response discarded
      ├─ 422 with /config/branches/0/transform/steps/i → plan without step i → retry → card shows the message
      ├─ other failure → banner + Retry; last good output kept
      └─ ok → PatchReplayer (event / event-list patches) → StudioRunResult → output, diff, examples, console, markers
```

17. P3 migration is `2080_transform_studio_library` (not `00NN`): description/archived_at, lookup `key_type` (CHECK), version
    `config_hash`/`change_note`, generated `row_count`, name indexes, `route_versions_config_gin`. `row_version` + triggers already
    existed (0002). Names are global references: create refuses a name used by any team (409 `/name`); the resolver picks the
    oldest object of a name.
18. Publish = schema check → `EngineStepCompiler` (one-branch check route, JS sandbox in test scope, library resolver for nested
    library steps; issue paths mapped to `/steps/i`) → sha256 of canonical JSON → `AppendVersion`. Archived objects get no versions (409).
19. Lookup imports: JSON `{rows, changeNote}` or a `text/csv` body (`?changeNote=`); per-line issues are a 422 with paths
    `/lines/{n}` (FE `LibraryImportError`), > 50k rows / 5 MB a 413. CSV: header with a `key` column; one more column is a text
    value, several become an object.
20. `StoreLibraryResolver` (`internal/library/resolver`) is wired into test-case runs, publish checks, routes' compile validation
    (`assembly.Ports.LibraryResolver`) and the preview service (`PipelineBuilder.Library`). The worker runtime still compiles
    without it (route-runtime has no Postgres; needs a control-plane resolver).
21. Transform test cases reuse `testcases` with a `TargetSource` per owner kind; `/transforms/{id}/test-cases` read with
    transform.read, write with library.write; `run` takes `{steps}` or the latest version.
22. Infer mapping only proposes: steps are checked like assistant proposals and returned with `issues`; the FE never applies
    them without Accept. The assistant limit is platform-core's Redis GCRA (`assist:<user>`, fails open); proposals carry ≤ 2
    samples masked by live-preview's `AssistSampleMasker`.

```
User clicks "Update" on an "Update available" badge (P3)
└─ StepsPane: LibraryLatest loads GET /transforms?names= & /lookup-tables?names=   [async, once per name set]
   ├─ pinned < latest → badge on the card (library step or named-table lookup)
   └─ Update → step.version = latest (StepListEditor.patch) → preview + test cases re-run; nothing changes otherwise
```

## 10. Open questions (remaining)
1. Assist endpoint paths `/assist/steps|explain|export-js` are new (not in SysDesign §6). `/preview/infer-mapping` keeps the spec path but is served by cmd/api. Is that OK?
