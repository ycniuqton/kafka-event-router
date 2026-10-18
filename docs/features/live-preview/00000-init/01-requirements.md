# Live Preview — Requirements (00000-init)

Sources: UI spec §9 (+ §7 right zone, §7.2 quick filter, §12 errors), SysDesign §10, §11 (audit/masking), §13.

## Business intent
Users never guess what a rule does: they see it. Every builder/studio edit is shown instantly on
real events from the source topic, using the exact production engine (`pkg/engine`), without
touching the route's consumer position or the real destination.

## Functional requirements
Sample loading
- FR-1 [MVP] Load the latest N events (default 50, max 1,000) from the route's source connection + topics,
  with manual partition assignment and **no consumer group** (never commits, never disturbs running routes).
  The same call with `count=20` serves connections' topic Peek (no route needed).
- FR-2 [MVP] Load events from a time range (`ListOffsets` by timestamp), merged by timestamp.
- FR-3 [MVP] Upload a JSON file (array or NDJSON) of events to create a session (empty dev topics);
  allowed for any editor even without a source connection.
- FR-4 [MVP] Hard limits per load: 1,000 events, 10 MB, 10 s; exceeding a limit truncates and says why.
- FR-5 [MVP] Samples are kept in a **sample session** (Redis, sliding TTL 1 h, owner-bound) so edits re-run
  without re-reading Kafka; expired sessions are reported so the UI reloads transparently.
- FR-6 [MVP] Text search across loaded samples (key, value, headers).
- FR-7 [MVP] Field schema inference: tree of paths with types, frequency and top-20 distinct values for
  low-cardinality fields; feeds field picker, value dropdowns ("OrderPaid — 12%") and JS autocomplete.
- FR-8 [P2] Avro/Protobuf samples decoded through schema registry; schema-registry fields merged into the
  inferred schema; format override per session.
- FR-9 [P3] Demo sample source: synthetic `order-events` for the demo workspace, no Kafka needed.

Preview run
- FR-10 [MVP] `POST /preview/run`: compile a **draft** route config, run every sample with tracing, return
  per event: final status (passed / filtered / error / dropped), per-filter-node result, per-step output
  snapshot as JSON patches vs previous step, logs, timings, encoded output (key, headers, size).
- FR-11 [MVP] Run on inline events (no session) for test cases, DLQ "open in studio" and Studio steppers.
- FR-12 [MVP] Compile/validation errors returned as RFC 7807 with JSON Pointer `path`; type-coercion warnings
  (e.g. `"100"` vs number) surfaced per node.
- FR-13 [MVP] Summary counts: "500 loaded · 37 pass · 461 filtered · 2 errors".
- FR-14 [P2] Per-branch results when a route has branches.

Sample panel (UI)
- FR-15 [MVP] Right zone of the builder, always visible, collapsible; shows events as they look
  **after the currently selected builder step** (source / filter / transform step / destination).
- FR-16 [MVP] Three views: Pretty (collapsible tree + type icons), Table (row per event, column per
  field), Raw (exact bytes as text + key + headers).
- FR-17 [MVP] Status per event: ✓ passed (with output), ⊘ filtered (failed condition highlighted),
  ⚠ error (message, step); status colours per UI §12.
- FR-18 [MVP] "Why was this filtered?": selecting a filtered event highlights each filter row
  green/red for that event in the filter builder.
- FR-19 [MVP] Auto re-run on draft change (debounced) and on `Ctrl+Enter`; stale runs cancelled.
- FR-20 [MVP] Value context actions emitted to owners: "Keep only events where …" (routes filter),
  Mask / Rename / Remove / Use as key (transform-studio click-to-transform).
- FR-21 [MVP] **Test** button: full route (filter + transform + output validation) on loaded samples.
- FR-22 [P2] Test run may write outputs to `router.test.<route>.<timestamp>` on the destination
  connection; deleted after 24 h; never the real destination.

Data protection
- FR-23 [MVP] Every sample load/view is audited (`samples.view`; raw/unmasked views flagged).
- FR-24 [P2] Masking: Viewers always get masked values; other callers get masked values only on
  connections marked `sensitive` (access-control `SampleVisibility` decides). Masking keeps types
  (strings → masked string, numbers → 0) and happens before values leave the backend; reused by AI assist (P3).

## Non-functional / constraints
- NFR-1 Parity: preview output == worker output for same config + events (shared `pkg/engine`,
  same codecs, same sandbox limits); release gate (SysDesign §13).
- NFR-2 Isolation: separate `cmd/preview` deployment; bounded concurrent runs per instance (429 when
  full); per-run deadline; a heavy preview never touches workers.
- NFR-3 Latency: re-run of 50 samples on a recipe route p95 < 300 ms; 1,000 samples < 2 s.
- NFR-4 Response size: step snapshots as RFC 6902 patches; panel paginates events.
- NFR-5 Security: API is the only public entry (OIDC session, team-scoped authz on connection);
  preview service internal-only (mTLS); secrets fetched per §11, never logged; payloads never logged.
- NFR-6 Determinism: frozen `ctx.now`, seeded random — repeat runs give identical traces.
- NFR-7 Coding rules (.devlens/rules.md): base interface + Manager per extensible domain (sample
  sources, session stores, maskers), config-selected implementations, enums for every literal.

## Out of scope
- Topic browser and the Peek UI (connections; Peek calls this API), live tail (monitoring), DLQ browsing (dead-letters).
- Transform by example `/preview/infer-mapping`, AI assist, saved test cases CRUD (transform-studio).
- Filter builder and studio editors themselves (routes, transform-studio) — they consume panel outputs.
- Engine semantics, trace content model, codecs (pipeline-engine); JS limits (js-sandbox).

## Dependencies
- pipeline-engine: `Compile`, `Pipeline.Process` with `Trace`, codecs, generated TS config types.
- js-sandbox: instance pools, `ctx.log` capture, determinism.
- connections: `pkg/kafka` client factory per security mode, admin ops, `connections.sensitive` (read only; owned by connections, migration 0004).
- platform-core: auth, problem+path, audit middleware, Redis client, internal mTLS, secrets fetch.
- access-control: `SampleVisibility` (Viewer always masked; sensitive connection masked), role → permission mapping.
- routes / transform-studio: host the panel; consume filter-node results and value actions.
- monitoring: demo workspace data generator (P3).
