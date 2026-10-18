# Transform Studio — Requirements (00000-init)

Sources: UI spec §8 (8.1–8.8), §13 phases; SysDesign §4 (library tables), §6 (Library API,
`/preview/infer-mapping`), §7.6, §14 milestones. Phase tags: MVP / P2 / P3.

## Business intent
Let anyone reshape Kafka events by pointing at real sample data, so they don't have to write code.
Steps are visual (Mapper), recipes (menu forms) or JavaScript, mixed freely in one ordered
pipeline. The Studio shows the result on real events immediately. Reusable transforms, lookup
tables, saved test cases and assistive "magic" features (by-example, AI assist, explain) make
transforms fast to build, safe to change and readable for reviewers.

## Functional requirements
### Studio shell
- FR-01 [MVP] Three-column layout: Input (JSON tree / table toggle, "Event N of M" stepper, type icons), Steps (cards), Output.
- FR-02 [MVP] Step cards: drag to reorder, enable/disable toggle, ⋯ menu (duplicate, delete), inline settings when selected, unsaved-change dot.
- FR-03 [MVP] Output shows the event after all steps or after the selected step ("show output at this step").
- FR-04 [MVP] Diff toggle: added green, removed red, changed amber. Key, headers and output size in bytes are shown below.
- FR-05 [MVP] Every edit re-runs the pipeline on the loaded samples through the preview service (same engine as workers).
### Recipe steps (Level 2)
- FR-10 [MVP] Grouped, searchable "+ Add step" menu with all recipe groups: Fields, Privacy, Types & formats, Values, Logic, Structure, Kafka.
- FR-11 [MVP] Each recipe is a small form (no code). The condition in If/else and Drop-if reuses the filter condition builder.
- FR-12 [MVP] Each recipe card shows a before → after mini example taken from the current sample.
- FR-13 [MVP] Lookup recipe: inline table edited as a grid or uploaded from CSV (named, versioned tables are P3).
- FR-14 [MVP] Click-to-transform: right-click an Input value → Mask / Rename / Remove / Use as key adds the matching recipe.
- FR-15 [P2] "Export to JS": convert selected recipe steps into an equivalent JS step.
### JavaScript step (Level 3)
- FR-20 [MVP] Monaco editor in the step card, expandable to full screen, with `transform(event, ctx)` as the template.
- FR-21 [MVP] Autocomplete of `event.value.*` from the field schema inferred from samples (types included), plus typed `ctx` helpers.
- FR-22 [MVP] "Run on sample" runs on all samples. Errors appear inline on the exact line with the failing event and a "jump to event" action.
- FR-23 [MVP] Console tab shows `ctx.log()` output per event.
- FR-24 [MVP] Snippets menu: drop if field missing, split array, rename keys to camelCase, and others.
- FR-25 [MVP] Info panel states the safety rules (§8.8): sandbox, time/memory limits, determinism, schema validation.
### Visual Mapper (Level 1)
- FR-30 [P2] Start from: copy input, empty, or paste an example output (the paste option uses FR-50 in P3).
- FR-31 [P2] Drag input field → output tree, with a connector line on hover. Users can rename, remove, and nest or un-nest fields in the output tree.
- FR-32 [P2] Value sources: From field, Fixed value, Formula, From metadata (topic, partition, offset, timestamp, route name).
- FR-33 [P2] Formula bar with autocomplete of fields and functions and a live result for the current event.
- FR-34 [P2] Unmapped input fields are listed in grey. The Mapper is persisted as one `map` step.
### Saved test cases
- FR-40 [P2] Pin a sample as a test case and lock its current output (or filtered/dropped outcome) as expected.
- FR-41 [P2] Every edit re-runs all test cases and shows pass/fail badges. A failing case shows its diff.
- FR-42 [P2] Failing test cases block Deploy (validation error), and warn on Save draft and Rollback. Test cases run in `cmd/api`; the rule itself is a routes validator calling `TestCaseService`.
### Magic features
- FR-50 [P3] Transform by example: paste the desired output and get proposed rename/move/set/remove steps with per-mapping confidence.
- FR-51 [P3] AI assist: a prompt produces proposed steps (recipes preferred over JS). They are shown as a diff and accepted or rejected. Nothing is applied silently.
- FR-52 [P3] Explain: a plain-language summary under a JS step (LLM provider) and under a recipe chain (rule-based, deterministic).
- FR-53 [P3] The AI goes through a provider-agnostic `LLMProvider`. Only a deterministic mock provider ships now.
### Transform library and lookup tables
- FR-60 [P3] Save steps as a library transform (name, description), with immutable versions (`mask-customer-pii v3`).
- FR-61 [P3] Use a library transform as a single `library` step pinned to a version. An "Update available" badge appears when a newer version exists. Steps never update automatically.
- FR-62 [P3] Library page: list/search transforms and lookup tables, see versions and usages (routes per version), and manage test cases per transform.
- FR-63 [P3] Named, versioned lookup tables (grid edit + CSV/JSON import), used by the lookup recipe and `ctx.lookup`.

## Non-functional / constraints
- NFR-1 Preview re-run p95 < 500 ms for 50 samples and typical steps (debounced 300 ms in the UI).
- NFR-2 Assist and infer only produce config. Proposals pass the normal route validator before they are shown.
- NFR-3 AI prompts carry at most 2 sample events and the field schema. Values are masked when the connection is sensitive.
- NFR-4 Library and lookup versions are immutable. A version referenced by a route can't be deleted.
- NFR-5 Lookup table ≤ 50,000 rows / 5 MB per version. CSV import errors are reported per line.
- NFR-6 Tested on a 1280 px screen with the sample panel collapsible; below 1440 px the Output column and the sample panel share the right zone as tabs. Keyboard reachable. Light and dark tokens.
- NFR-7 Follows the devlens coding rules: provider, matcher, exporter and importer families each have a base interface and a Manager, and implementations are selected by config.
- NFR-8 Every mutation is permission-checked (editor+) and audited. Viewers have read-only access to the Studio.

## Out of scope
- Real LLM providers (OpenAI, Anthropic and others). Only the interface and the mock are in scope.
- Engine semantics of steps and formulas, and the JS sandbox runtime (owned elsewhere, see below).
- Sample loading, the sample panel and `/preview/run` (live-preview). Route builder shell and deploy (routes).
- Library marketplace. (The library is readable by all teams — open read; writes stay team-scoped.)

## Dependencies
- systems/pipeline-engine: step kinds and JSON Schema, `map` step, formulas, library ref resolution, generated TS types.
- systems/js-sandbox: `ctx` helpers, limits, compile errors with line/column.
- systems/platform-core: API module registry, RFC7807, If-Match, cursor pagination, audit middleware, migrations.
- features/live-preview: sample sessions, field schema inference, `/preview/run` traces, masking for sensitive connections.
- features/routes: builder shell (Transform card), draft save, deploy validation registry.
- features/access-control: roles and RBAC policy for library and assist actions.
