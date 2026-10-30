# Kafka Event Router — Acceptance test cases

Derived from the *Product & UI Specification* and the *System & Functional Design* (Oct 13, 2026), the
feature/system requirements in `docs/**/01-requirements.md` and the existing E2E suites (`web/e2e`, `test/e2e`).

- **ID** `TC-<AREA>-<n>` · **Pri** P0 = release blocker, P1 = important, P2 = nice to have ·
  **Phase** MVP / P2 / P3 as tagged in UI spec §13.
- **Automated by**: spec file under `test/acceptance/specs/` › test named after the case id (`npx playwright test -g TC-XXX-n`),
  or `manual` with the reason it is not automated.
- **Local** / **Staging** are the status of the last run on each target (`ACC_TARGET=local|staging`), written by
  `npm run catalogue` from `report/results.json` into the column of the run's target: `pass`, `fail`, `skip`
  (precondition missing on the target), `manual`, `not run`. Failures that are not fixed are explained in
  [Known failures](#known-failures).
- Staging skips (19): Dex has one static user, so the multi-role cases (viewer / editor / approver: TC-APR-01…13, 19,
  TC-PRV-13, TC-AUD-04, TC-SET-01/02) cannot sign in as other users there; TC-SEC-02 scans the local worker log only.
  These pass on local. TC-SEC-11 (dev login refused) is staging-only and is the local skip.

<!-- run-summary:start -->
**Last run:** 2026-10-18 11:43:44 UTC · **target:** `local`

| Target | Last run | Cases | Automated | Manual | Pass | Fail | Skip | Not run |
|---|---|---|---|---|---|---|---|---|
| local | 2026-10-18 11:43:44 UTC | 239 | 204 | 35 | 202 | 1 | 1 | 0 |
| staging | 2026-10-16 15:22:07 UTC | 239 | 204 | 35 | 161 | 1 | 19 | 23 |
<!-- run-summary:end -->

## Connections (UI spec §5)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-CONN-01 | P0 | MVP | Create a plaintext connection | Admin signed in | POST /connections (name, env dev, bootstrap, plaintext) then GET it | 201 + ETag; name, env, bootstrap, security returned; used by 0 routes | api/connections.spec.ts › TC-CONN-01 | pass | pass |
| TC-CONN-02 | P0 | MVP | Test checklist passes with live ticks | Reachable cluster | POST /connections/{id}/test | NDJSON steps brokers_reachable → authenticated → list_topics passed; result ok, health healthy and stored | api/connections.spec.ts › TC-CONN-02 | pass | pass |
| TC-CONN-03 | P0 | MVP | Unreachable broker gives a plain-language reason and hint | — | Create connection to 127.0.0.1:1, test it | brokers_reachable failed with reason unreachable, message and hint; health unreachable | api/connections.spec.ts › TC-CONN-03 | pass | pass |
| TC-CONN-04 | P0 | MVP | Passwords are write-only | — | see TC-SEC-01 | Never returned, shown as set | api/audit-settings-security.spec.ts (TC-SEC-01) | pass | pass |
| TC-CONN-05 | P1 | MVP | PATCH without secrets keeps the stored password | SASL/PLAIN connection with password | PATCH description only | 200; secretFields.password still set | api/connections.spec.ts › TC-CONN-05 | pass | pass |
| TC-CONN-06 | P1 | MVP | Mode-specific fields are validated | — | Create SASL/PLAIN without username | 422 problem naming the username field | api/connections.spec.ts › TC-CONN-06 | pass | pass |
| TC-CONN-07 | P1 | MVP | Delete is blocked while routes use the connection | Route uses connection | DELETE /connections/{id} | 409 listing the route; connection still used by 1 | api/connections.spec.ts › TC-CONN-07 | pass | pass |
| TC-CONN-08 | P0 | MVP | Browse topics with partitions | Topic with 3 partitions | GET /connections/{id}/topics?q= | Topic listed with partitions 3 | api/connections.spec.ts › TC-CONN-08 | pass | pass |
| TC-CONN-09 | P1 | MVP | Create topic from a connection (if allowed) | allowTopicCreate on / off | POST topics twice; POST on locked connection | 201, then 409 duplicate; 403 when not allowed | api/connections.spec.ts › TC-CONN-09 | pass | pass |
| TC-CONN-10 | P1 | MVP | Advanced raw client properties are allow-listed | — | Create with sasl.jaas.config; create with client.id | 422 for the unsupported one; allowed one stored | api/connections.spec.ts › TC-CONN-10 | pass | pass |
| TC-CONN-11 | P2 | MVP | Bootstrap chips split a pasted comma list | Connection panel open | Paste `a:9092,b:9092,c:9092` in Brokers | Three chips; list shows first + "+2" | manual (clipboard paste not scripted) | manual | manual |
| TC-CONN-12 | P1 | MVP | Test an unsaved draft | — | POST /connections/test with a draft body | Checklist passes; nothing is saved | api/connections.spec.ts › TC-CONN-12 | pass | pass |
| TC-CONN-13 | P1 | MVP | Optimistic concurrency on PATCH | Connection exists | PATCH with current, then stale If-Match, then none | 200, 412, 428 | api/connections.spec.ts › TC-CONN-13 | pass | pass |
| TC-CONN-14 | P0 | MVP | UI: 4-step side panel → Save & test → Healthy row | — | Connections → + New connection → Basics, Brokers, Security None, Registry → Save & test | "All checks passed."; row shows Healthy | ui/connections-routes.ui.spec.ts › TC-CONN-14 | pass | pass |
| TC-CONN-15 | P1 | MVP | UI: Peek shows the latest events | Topic with one event | Connections → Topics → search → Peek | The produced event is shown in the event viewer | ui/connections-routes.ui.spec.ts › TC-CONN-15 | pass | pass |
| TC-CONN-16 | P2 | P2 | Schema registry credentials are write-only | — | Create with registry URL + basic auth secret; GET | schemaRegistry.secret = set; secret value absent | api/connections.spec.ts › TC-CONN-16 | pass | pass |
| TC-CONN-17 | P2 | MVP | Security segmented control shows only the chosen mode's fields; .pem drag-and-drop | Panel open | Switch None → SASL/SCRAM → mTLS → OAuth; drop a .pem | Only matching fields visible; certificate text filled | manual (file drag-and-drop) | manual | manual |
| TC-CONN-18 | P1 | MVP | List filters by environment and text | staging connection | GET ?q=, ?env=prod, ?env=qa | Only the matching one; empty for prod; 4xx for unknown env | api/connections.spec.ts › TC-CONN-18 | pass | pass |
| TC-CONN-19 | P1 | MVP | Optional "can write" check produces a marked record | Existing topic | Test with readTopic + writeTopic; consume | can_read / can_write passed; record carries header x-router-test | api/connections.spec.ts › TC-CONN-19 | pass | pass |
| TC-CONN-20 | P1 | MVP | Delete disabled in the UI with a tooltip listing routes | Connection used by a route | Hover Delete on the row | Disabled; tooltip names the route | manual (hover tooltip) | manual | manual |

## Routes list (UI spec §6)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-RTL-01 | P0 | MVP | A created route is a paused draft listed like a sentence | Connection + topics | POST /routes with draft; GET list | status draft, desired paused, source connection/topic and destination topic in the row | api/routes.spec.ts › TC-RTL-01 | pass | pass |
| TC-RTL-02 | P1 | MVP | Search and status filter | Draft route | GET ?q= (any case), ?status=draft / running | Case-insensitive match; status filter applies | api/routes.spec.ts › TC-RTL-02 | pass | pass |
| TC-RTL-03 | P1 | MVP | Filter summary and transform indicator | Routes with no / recipe / JS steps | GET list | transform none / visual / js; filter headline + "more" count | api/routes.spec.ts › TC-RTL-03 | pass | pass |
| TC-RTL-04 | P1 | MVP | Duplicate | Route | POST /routes/{id}/duplicate | 201 paused copy named `<name>-copy…` with the same draft | api/routes.spec.ts › TC-RTL-04 | pass | pass |
| TC-RTL-05 | P1 | MVP | Archive and unarchive | Route | DELETE, list, list status=archived, unarchive | Hidden by default; listed as archived; back as paused | api/routes.spec.ts › TC-RTL-05 | pass | pass |
| TC-RTL-06 | P1 | MVP | Bulk actions | Two drafts | bulk add_tag, bulk pause | Per-id results; tag filter finds both; pause of a draft reports ok=false | api/routes.spec.ts › TC-RTL-06 | pass | pass |
| TC-RTL-07 | P2 | P2 | Bulk export as YAML | Two drafts | bulk export yaml | One multi-document artifact containing both | api/routes.spec.ts › TC-RTL-07 | pass | pass |
| TC-RTL-08 | P1 | MVP | UI: + New route chooser and sentence rows | A draft route | Routes → + New route; then search the route | Four cards (Filter & forward, Publish to partner, Merge topics, Blank route); row shows source → destination and Draft badge | ui/connections-routes.ui.spec.ts › TC-RTL-08 | pass | pass |
| TC-RTL-09 | P2 | MVP | Empty state explains routes and offers + New route | Workspace without routes | Open Routes | "No routes yet. A route moves events…" + New route | manual (needs an empty workspace) | manual | manual |
| TC-RTL-10 | P2 | MVP | Table / card view toggle | Routes exist | Toggle view | Same routes as cards | manual | manual | manual |
| TC-RTL-11 | P2 | MVP | Keyboard shortcuts N, /, Ctrl+Enter, Ctrl+S | — | Press keys on Routes / builder | New route chooser, search focus, run on samples, save draft | manual | manual | manual |
| TC-RTL-12 | P1 | MVP | Throughput, match rate and lag columns | Running route with traffic and metrics | Open Routes | Sparkline, match %, lag per row | manual (local Prometheus scrapes staging only) | manual | manual |

## Route builder and filter (UI spec §7)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-BLD-01 | P0 | MVP | Validation issues carry JSON pointer paths | Destination topic missing | POST /validate mode deploy | valid=false; error at /branches/0/destination/… with a readable message | api/routes.spec.ts › TC-BLD-01 | pass | pass |
| TC-BLD-02 | P0 | MVP | Nothing deploys without validation | Invalid draft | Save draft; deploy | Save 200 (never blocked); deploy 422 with issues; no active version | api/routes.spec.ts › TC-BLD-02 | pass | pass |
| TC-BLD-03 | P1 | P3 | Exactly-once only on same-cluster routes | Two connections | Validate exactly_once across connections | Error about delivery mode | api/routes.spec.ts › TC-BLD-03 | pass | pass |
| TC-BLD-04 | P0 | MVP | Native filter operators | Samples | Preview eq, gt, in, contains, regex, exists, between | Each matches exactly the expected events | api/routes.spec.ts › TC-BLD-04 | pass | pass |
| TC-BLD-05 | P1 | MVP | Nested AND / OR / NOT groups | Samples | Preview nested group | Expected pass/filter per event | api/routes.spec.ts › TC-BLD-05 | pass | pass |
| TC-BLD-06 | P1 | MVP | Expression mode | Samples | Preview expr `amount > 100 && currency === "USD"` | Only the matching event passes | api/routes.spec.ts › TC-BLD-06 | pass | pass |
| TC-BLD-07 | P1 | MVP | Type coercion with a preview warning | "150" vs number | Preview gt 100 | Passes; response mentions coercion | api/routes.spec.ts › TC-BLD-07 | pass | pass |
| TC-BLD-08 | P1 | MVP | JS must compile | Broken JS step | Validate deploy | Error at /branches/0/transform/steps/0 | api/routes.spec.ts › TC-BLD-08 | pass | pass |
| TC-BLD-09 | P1 | MVP | Missing destination topic + createTopic | createTopic partitions 2 | Validate; deploy | Valid; topic exists after deploy | api/routes.spec.ts › TC-BLD-09 | pass | pass |
| TC-BLD-10 | P0 | MVP | UI: build source → filter → destination, deploy, running | Connection + topics | Builder (Filter & forward) → upload samples → quick filter → destination → Save draft → Review & deploy → Deploy | Route running; matching events delivered on Kafka | ui/connections-routes.ui.spec.ts › TC-BLD-10 | pass | pass |
| TC-BLD-11 | P2 | MVP | Unsaved changes dot on the affected card | Builder open | Edit a step | Dot on that card until Save draft | manual | manual | manual |
| TC-BLD-12 | P1 | MVP | Quick filter from data | Samples loaded | Click a value in the sample panel | Filter row `type = OrderPaid` added | ui/connections-routes.ui.spec.ts (TC-BLD-10) | pass | pass |
| TC-BLD-13 | P1 | MVP | Live match counter | Filter + samples | Edit filter | "Matches 2 of 3 sample events" | ui/connections-routes.ui.spec.ts (TC-BLD-10) | pass | pass |
| TC-BLD-14 | P1 | P2 | Visual rows ↔ expression conversion | Filter with rows | Switch to Expression and back | Expression generated; complex expression says it cannot convert back | manual | manual | manual |
| TC-BLD-15 | P1 | MVP | Consumer group auto-generated | Draft without consumerGroup | Create; read draft | `router.<route name>` | api/routes.spec.ts › TC-BLD-15 | pass | pass |
| TC-BLD-16 | P1 | MVP | Field picker from samples and schema with type + example | Samples loaded | Open field picker | Tree of fields with type icons and examples; value dropdown with frequencies | manual (covered by web/e2e live-preview) | manual | manual |
| TC-BLD-17 | P2 | P2 | Topic pattern (regex) and specific offsets saved | — | Source: Pattern, Offsets, Avro override | Saved in draft; validation flags missing registry / offsets | manual (covered by web/e2e routes-p2) | manual | manual |

## Transform Studio (UI spec §8)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-TS-01 | P0 | MVP | Rename | Sample | Preview rename cust_id → customerId | Value moved, old field gone | api/transform-studio.spec.ts › TC-TS-01 | pass | pass |
| TC-TS-02 | P0 | MVP | Mask full / last 4 / email | Sample | Preview mask styles | `****`, `************1111`, `j***@mail.com` (spec §8.4) | api/transform-studio.spec.ts › TC-TS-02 | pass | pass |
| TC-TS-03 | P1 | MVP | Hash SHA-256 with a secret salt | Sample | Preview hash twice | 64-hex, stable, not the unsalted digest | api/transform-studio.spec.ts › TC-TS-03 | pass | pass |
| TC-TS-04 | P1 | MVP | Remove and keep only | Sample | Preview | Fields removed / only listed kept | api/transform-studio.spec.ts › TC-TS-04 | pass | pass |
| TC-TS-05 | P1 | MVP | Copy, move / nest, flatten | Sample | Preview | Copied; moved into object; nested object flattened to dotted keys | api/transform-studio.spec.ts › TC-TS-05 | pass | pass |
| TC-TS-06 | P1 | MVP | Set value: constant, field, metadata, formula | Sample | Preview concat / round / upper formulas | Values as specified | api/transform-studio.spec.ts › TC-TS-06 | pass | pass |
| TC-TS-07 | P1 | MVP | Convert type, round, case, format date | Sample | Preview | "1", 12.3, "vn", "2026-10-16" | api/transform-studio.spec.ts › TC-TS-07 | pass | pass |
| TC-TS-08 | P1 | MVP | Lookup table with default; default if missing | Two events | Preview | PAID / UNKNOWN; EUR default only when missing | api/transform-studio.spec.ts › TC-TS-08 | pass | pass |
| TC-TS-09 | P1 | MVP | If / else and drop if | Three events | Preview | APAC / OTHER; third dropped | api/transform-studio.spec.ts › TC-TS-09 | pass | pass |
| TC-TS-10 | P1 | MVP | Split array, wrap, unwrap | Sample with items | Preview | One event per item; envelope; unwrapped object | api/transform-studio.spec.ts › TC-TS-10 | pass | pass |
| TC-TS-11 | P1 | MVP | Kafka recipes: key and header ops | Event with headers | Preview set_key, header add/remove/rename/copy | Key from field; headers exactly as expected; header copied to payload | api/transform-studio.spec.ts › TC-TS-11 | pass | pass |
| TC-TS-12 | P1 | MVP | Redact by pattern | Free text with card / phone | Preview | Card and phone number redacted, other text kept | api/transform-studio.spec.ts › TC-TS-12 | pass | pass |
| TC-TS-13 | P0 | MVP | JS contract: event / null / array | Three events | Preview JS step | passed / dropped / split into two | api/transform-studio.spec.ts › TC-TS-13 | pass | pass |
| TC-TS-14 | P0 | MVP | JS error reported on the exact line with the event | Event without order | Preview JS that throws on line 3 | Status error; message; position (3:x) | api/transform-studio.spec.ts › TC-TS-14 | pass | pass |
| TC-TS-15 | P1 | MVP | ctx helpers identical to recipes | Sample | JS ctx.mask.email + ctx.hash vs recipes | Identical output | api/transform-studio.spec.ts › TC-TS-15 | pass | pass |
| TC-TS-16 | P1 | MVP | Console tab: ctx.log per event | Two events | Preview JS with ctx.log | Logs per event | api/transform-studio.spec.ts › TC-TS-16 | pass | pass |
| TC-TS-17 | P1 | MVP | Determinism | Same event twice | Preview Math.random / Date.now | Same values in both runs | api/transform-studio.spec.ts › TC-TS-17 | pass | pass |
| TC-TS-18 | P1 | MVP | Disabled step is skipped | Sample | Preview with enabled=false | No effect | api/transform-studio.spec.ts › TC-TS-18 | pass | pass |
| TC-TS-19 | P1 | P2 | Visual Mapper map step | Sample | Preview map (copy input, field source, remove, formula, metadata) | Output tree as designed | api/transform-studio.spec.ts › TC-TS-19 | pass | pass |
| TC-TS-20 | P2 | P2 | Export recipe steps to JS | Steps | POST /assist/export-js; preview both | One JS step; identical output | api/transform-studio.spec.ts › TC-TS-20 | pass | pass |
| TC-TS-21 | P1 | P3 | Transform by example | Input + desired output | POST /preview/infer-mapping; preview proposal | rename + set proposed with mappings; unmapped listed; output equals desired | api/transform-studio.spec.ts › TC-TS-21 | pass | pass |
| TC-TS-22 | P1 | P3 | AI assist proposes recipe steps (only when enabled) | AI assist off / on | POST /assist/steps | 403 when off; mask + rename, no JS, diff when on | api/transform-studio.spec.ts › TC-TS-22 | pass | pass |
| TC-TS-23 | P2 | P3 | Explain a step chain | AI assist on | POST /assist/explain | Plain-language summary mentioning the fields | api/transform-studio.spec.ts › TC-TS-23 | pass | pass |
| TC-TS-24 | P1 | P2 | Saved test cases pass / fail with diff | Route with rename | Pin case; run; run with changed config | 1 passed; then fail with diff | api/library-testcases.spec.ts › TC-TS-24 | pass | pass |
| TC-TS-25 | P1 | P2 | Failing test case blocks deploy | Failing case | Deploy | 422 mentioning the test case | api/library-testcases.spec.ts › TC-TS-25 | pass | pass |
| TC-TS-26 | P1 | P3 | Library transform as one pinned step | Library transform v1 | Preview `library` step | Same output as its steps | api/library-testcases.spec.ts › TC-TS-26 | pass | pass |
| TC-TS-27 | P1 | P3 | Update available, never automatic | v2 published; route pinned at v1 | Preview, usages, validate | v1 behaviour kept; latestVersion 2; route listed in usages; validation says an update exists | api/library-testcases.spec.ts › TC-TS-27 | pass | pass |
| TC-TS-28 | P2 | P3 | Versioned lookup table | Table v1 | Preview lookup step with ref | Value mapped | api/library-testcases.spec.ts › TC-TS-28 | pass | pass |
| TC-TS-29 | P1 | MVP | UI: recipe card before → after example and output | Uploaded sample | Studio → + Add step Mask (email), Rename | Card shows john@mail.com → j***@mail.com; output has customerId | ui/studio-monitoring.ui.spec.ts › TC-TS-29 | pass | pass |
| TC-TS-30 | P2 | MVP | UI: diff toggle colours, key / headers / size below output | Steps applied | Toggle Diff | Added green, removed red, changed amber; size in bytes | manual (colour check) | manual | manual |
| TC-TS-31 | P1 | MVP | UI: JS editor info panel shows safety rules and limits | — | Add JavaScript step → ⓘ | Rules (no network…) and "50 ms per event" | ui/studio-monitoring.ui.spec.ts › TC-TS-31 | pass | pass |
| TC-TS-32 | P1 | MVP | Click-to-transform from the Input panel | Sample | Right-click value → Mask this / Rename / Remove / Use as key | Matching recipe added | manual (context menu) | manual | manual |
| TC-TS-33 | P2 | MVP | Drag to reorder steps; duplicate / delete via ⋯ | Two steps | Drag; menu | Order and output change accordingly | manual (drag and drop) | manual | manual |
| TC-TS-34 | P2 | MVP | Snippets menu | JS step | Insert "drop if field missing" | Snippet inserted and runs | manual | manual | manual |
| TC-TS-35 | P1 | MVP | TypeError position is on its exact line | Event without order | Preview JS reading a property of undefined on line 2 | Position (2:x) | api/transform-studio.spec.ts › TC-TS-35 | fail | fail |
| TC-TS-36 | P2 | MVP | JS autocomplete knows sample fields | Samples | Type `event.value.` in Monaco | Real fields with types suggested | manual (Monaco) | manual | manual |

## Live preview and testing (UI spec §9)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-PRV-01 | P0 | MVP | Latest N without moving any consumer position | Six events on a topic | POST /preview/samples latest 50; list groups | Six samples; no consumer group owns offsets on the topic | api/live-preview.spec.ts › TC-PRV-01 | pass | pass |
| TC-PRV-02 | P0 | MVP | Upload JSON events | — | POST samples mode upload | Session ps_… created | api/live-preview.spec.ts › TC-PRV-02 | pass | pass |
| TC-PRV-03 | P1 | MVP | Hard limit 1,000 events | — | Request 5,000 | Refused (422) or clamped to ≤ 1,000 | api/live-preview.spec.ts › TC-PRV-03 | pass | pass |
| TC-PRV-04 | P0 | MVP | Per-event status and summary counts | Six events, filter + throwing JS | POST /preview/run | 6 loaded · 2 passed · 3 filtered · 1 error, per event in order | api/live-preview.spec.ts › TC-PRV-04 | pass | pass |
| TC-PRV-05 | P1 | MVP | Why was this filtered | AND of two conditions | Run | Trace: first child true, second false, root false | api/live-preview.spec.ts › TC-PRV-05 | pass | pass |
| TC-PRV-06 | P1 | MVP | Field schema inference | Uploaded session | GET sessions/{id}/schema | Paths with types and top values (OrderPaid) | api/live-preview.spec.ts › TC-PRV-06 | pass | pass |
| TC-PRV-07 | P1 | MVP | Test run to a temporary topic | Draft route | POST /preview/test-runs writeToTopic | Outputs on router.test.<route>.<ts>; real destination untouched | api/live-preview.spec.ts › TC-PRV-07 | pass | pass |
| TC-PRV-08 | P1 | MVP | Search samples by text | Uploaded session | GET events?q= | Only the matching event | api/live-preview.spec.ts › TC-PRV-08 | pass | pass |
| TC-PRV-09 | P1 | MVP | Raw view: exact bytes, key, headers | Event with odd spacing + header | Load latest 1 | raw.value byte-exact; key; header; decoded value | api/live-preview.spec.ts › TC-PRV-09 | pass | pass |
| TC-PRV-10 | P1 | MVP | Load from a time range | Six events | Load mode time_range | Six samples | api/live-preview.spec.ts › TC-PRV-10 | pass | pass |
| TC-PRV-11 | P0 | MVP | Preview = production (parity) | — | see TC-RUN-02 | Identical outputs | api/route-runtime.spec.ts (TC-RUN-02) | pass | pass |
| TC-PRV-12 | P1 | MVP | UI: sample panel summary counts | Builder with uploaded samples + filter | Upload, quick filter | "3 loaded · 2 pass · 1 filtered · 0 errors" | ui/connections-routes.ui.spec.ts › TC-PRV-12 | pass | pass |
| TC-PRV-13 | P1 | P2 | Viewer gets masked samples | — | see TC-APR-11 | Masked | api/access-approvals.spec.ts (TC-APR-11) | pass | skip |
| TC-PRV-14 | P1 | P2 | Sample access is audited | — | see TC-AUD-03 | Audit entry | api/audit-settings-security.spec.ts (TC-AUD-03) | pass | pass |
| TC-PRV-15 | P2 | MVP | Pretty / Table / Raw views and "Why?" highlighting in the UI | Samples + filter | Switch views; click Why? | Three views; condition rows green / red | manual (covered by web/e2e live-preview) | manual | manual |

## Deploy, versions, rollback, YAML (UI spec §7.4, §11.1)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-DEP-01 | P0 | MVP | Deploy creates v1; route runs; deployment active after the safety window | Valid draft | Deploy with note; wait | 202 v1 origin deploy with hash + note; observed running; deployment active after ~60 s | api/deploy-versions.spec.ts › TC-DEP-01 | pass | pass |
| TC-DEP-02 | P0 | MVP | Versions newest first with author and note | Two deploys | GET versions | [2, 1]; v2 active; notes and author | api/deploy-versions.spec.ts › TC-DEP-02 | pass | pass |
| TC-DEP-03 | P1 | MVP | Readable diff per section | v1, v2 | GET versions/1/diff/2 | filter section as readable rows; transform section changed | api/deploy-versions.spec.ts › TC-DEP-03 | pass | pass |
| TC-DEP-04 | P0 | MVP | Rollback = new version, history kept | v1, v2 | POST rollback version 1 | v3 origin rollback basedOn 1 with v1 branches; history [3,2,1]; active v3 | api/deploy-versions.spec.ts › TC-DEP-04 | pass | pass |
| TC-DEP-05 | P1 | MVP | Active ↔ draft diff shows undeployed changes | Deployed + edited draft | GET diff active/draft | hasUndeployedDraft; diff mentions the change | api/deploy-versions.spec.ts › TC-DEP-05 | pass | pass |
| TC-DEP-06 | P1 | MVP | Deploy with stale If-Match | Draft saved after reading | Deploy with old ETag | 412 | api/deploy-versions.spec.ts › TC-DEP-06 | pass | pass |
| TC-DEP-07 | P1 | P2 | Export YAML with names, no ids or secrets | SASL connection with password | GET export yaml | Attachment `<name>.yaml`; connection name; no id; no password | api/deploy-versions.spec.ts › TC-DEP-07 | pass | pass |
| TC-DEP-08 | P1 | P2 | Import YAML: dry run, draft only | Exported YAML renamed | Import dryRun; import | Preview "created" without creating; then a draft with no active version | api/deploy-versions.spec.ts › TC-DEP-08 | pass | pass |
| TC-DEP-09 | P2 | P2 | Import upsert replaces an existing draft | Existing route | Import (create mode); import mode=upsert | skipped_exists; draft_updated with the new topic | api/deploy-versions.spec.ts › TC-DEP-09 | pass | pass |
| TC-DEP-10 | P1 | MVP | Pause / resume | Running route; a draft | Pause draft; pause / resume running | 409 for draft; desired + observed paused, then running | api/deploy-versions.spec.ts › TC-DEP-10 | pass | pass |
| TC-DEP-11 | P1 | MVP | UI: History tab and one-click rollback | v1, v2 | History → Roll back → Roll back to v1 | Versions listed with notes; v3 active | ui/connections-routes.ui.spec.ts › TC-DEP-11 | pass | pass |
| TC-DEP-12 | P1 | MVP | Review page reads as a sentence with diff and dry run | Edited route | Review & deploy | Sentence, diff vs running, validation, dry run results | manual (covered by web/e2e routes) | manual | manual |
| TC-DEP-13 | P1 | MVP | Automatic rollback when a new version errors in its first 60 s | Running v1 | Deploy v2 whose source fails at start | Deployment failed; auto_rollback version created | manual (needs a reproducible runtime start failure) | manual | manual |
| TC-DEP-14 | P1 | MVP | A compile failure on the worker keeps the old version | — | Force a version the worker cannot compile | Old version keeps running; deployment failed | manual (API validation blocks such versions) | manual | manual |
| TC-DEP-15 | P1 | MVP | PATCH needs If-Match | Route | PATCH without If-Match | 428 | api/deploy-versions.spec.ts › TC-DEP-15 | pass | pass |

## Approvals, roles and masking (UI spec §11.2)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-APR-01 | P0 | MVP | Viewer cannot create routes | Viewer user | POST /routes | 403 naming route.write / editor | api/access-approvals.spec.ts › TC-APR-01 | pass | skip |
| TC-APR-02 | P0 | MVP | Only admins manage connections | Viewer, editor | POST /connections | 403 for both | api/access-approvals.spec.ts › TC-APR-02 | pass | skip |
| TC-APR-03 | P0 | MVP | Editor deploys to non-production directly | Editor; dev connection | Create + deploy | Not pending approval | api/access-approvals.spec.ts › TC-APR-03 | pass | skip |
| TC-APR-04 | P0 | P2 | Production deploy becomes an approval request | Editor; prod connection; team requires approval | Deploy | pending_approval; nothing live | api/access-approvals.spec.ts › TC-APR-04 | pass | skip |
| TC-APR-05 | P0 | P2 | Requester can never approve their own request | Pending request | Requester approves | 403 | api/access-approvals.spec.ts › TC-APR-05 | pass | skip |
| TC-APR-06 | P0 | P2 | Approver approves → version activated; one decision wins | Pending request | Approver approves twice | 200 then 409; active version 1 | api/access-approvals.spec.ts (TC-APR-05) | pass | skip |
| TC-APR-07 | P1 | P2 | Reject needs a note; version number consumed | Pending request | Reject without / with note; request again | 422; rejected; next request is v2 | api/access-approvals.spec.ts › TC-APR-07 | pass | skip |
| TC-APR-08 | P1 | P2 | Requester withdraws | Pending request | POST withdraw | cancelled, reason withdrawn | api/access-approvals.spec.ts › TC-APR-08 | pass | skip |
| TC-APR-09 | P1 | P2 | Newer request cancels the older | Two requests | Read the first | No longer pending; cannot be approved | api/access-approvals.spec.ts (TC-APR-08) | pass | skip |
| TC-APR-10 | P1 | P2 | Comments; canDecide per user | Pending request | Approver comments; both read | Comment visible; requester cannot decide, approver can | api/access-approvals.spec.ts › TC-APR-10 | pass | skip |
| TC-APR-11 | P0 | P2 | Masking: viewers always, editors on sensitive connections | Plain + sensitive connection | Load samples as viewer / editor | Viewer masked; editor raw on plain, masked on sensitive | api/access-approvals.spec.ts › TC-APR-11 | pass | skip |
| TC-APR-12 | P1 | MVP | Viewer reads routes, history, metrics | Deployed route | GET as viewer; pause | 200s; pause 403 | api/access-approvals.spec.ts › TC-APR-12 | pass | skip |
| TC-APR-13 | P1 | MVP | /auth/me embeds permissions | Viewer | GET /auth/me | role viewer; route.read, no route.write | api/access-approvals.spec.ts › TC-APR-13 | pass | skip |
| TC-APR-14 | P1 | P2 | Connection grants between teams | Second team | Validate route on another team's connection; grant; validate | Connection issue; gone after the grant | api/access-approvals.spec.ts › TC-APR-14 | pass | pass |
| TC-APR-15 | P1 | MVP | Last admin cannot be removed | Team with one admin | DELETE own binding | 409 last_admin | manual (would demote the shared dev admin) | manual | manual |
| TC-APR-16 | P1 | P2 | UI: Request approval button and review page | Editor + approver | Deploy from builder; approve in review page | Button reads Request approval; review shows diff; approval activates | manual (covered by web/e2e approvals, routes-p2) | manual | manual |
| TC-APR-17 | P1 | MVP | Read-scoped API token cannot write | Token scope read | GET, POST with bearer only | 200, 403 | api/access-approvals.spec.ts › TC-APR-17 | pass | pass |
| TC-APR-18 | P1 | MVP | Revoked token stops working | Token | Use; revoke; use | 200, 204, 401 | api/access-approvals.spec.ts › TC-APR-18 | pass | pass |
| TC-APR-19 | P1 | MVP | Disabling a user revokes sessions | Signed-in viewer | Disable; call API | 401 within seconds | api/access-approvals.spec.ts › TC-APR-19 | pass | skip |
| TC-APR-20 | P2 | P2 | Approvers get a bell notification with the diff | Pending request | Open bell as approver | Notification links to the review page | manual | manual | manual |

## Route runtime (SysDesign §8–§9)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-RUN-01 | P0 | MVP | Filter & forward end to end | Running route, filter OrderPaid | Produce 6 | Exactly the 3 matching keys delivered | api/route-runtime.spec.ts › TC-RUN-01 | pass | pass |
| TC-RUN-02 | P0 | MVP | Parity: worker output = preview output | Rename + mask + formula | Preview, then produce | Identical values | api/route-runtime.spec.ts › TC-RUN-02 | pass | pass |
| TC-RUN-03 | P0 | MVP | At-least-once across pause / resume | Running route | 20 events; pause; 20 more; resume | All 40 keys delivered | api/route-runtime.spec.ts › TC-RUN-03 | pass | pass |
| TC-RUN-04 | P0 | MVP | Ordering per partition and key | 3 partitions | Produce 10 per key | Per key increasing, one partition per key | api/route-runtime.spec.ts › TC-RUN-04 | pass | pass |
| TC-RUN-05 | P0 | MVP | JS error → DLQ with router.* headers | Throwing JS | Produce bad + ok | ok delivered; DLQ record has original key/value/headers and all router.* headers | api/route-runtime.spec.ts › TC-RUN-05 | pass | pass |
| TC-RUN-06 | P0 | MVP | Decode error → DLQ; route continues | — | Produce `{not json` + valid | Valid delivered; dead letter type decode | api/route-runtime.spec.ts › TC-RUN-06 | pass | pass |
| TC-RUN-07 | P1 | MVP | onError skip | Throwing JS, skip | Produce bad + ok | ok delivered; no DLQ record | api/route-runtime.spec.ts › TC-RUN-07 | pass | pass |
| TC-RUN-08 | P1 | MVP | onError pause halts on the first failure | Throwing JS, pause | Produce ok, bad, ok | Route error/paused; only the first ok delivered | api/route-runtime.spec.ts › TC-RUN-08 | pass | pass |
| TC-RUN-09 | P1 | MVP | JS timeout retried, then DLQ | Loop on slow events; maxRetries 1 | Produce slow + fast | fast delivered; dead letter type timeout | api/route-runtime.spec.ts › TC-RUN-09 | pass | pass |
| TC-RUN-10 | P0 | MVP | Hot reload without loss | Running v1 | Deploy v2; produce more | All 10 delivered; later ones carry v2 | api/route-runtime.spec.ts › TC-RUN-10 | pass | pass |
| TC-RUN-11 | P1 | P2 | Branches to separate destinations | Two branches | Produce 4 | Each branch receives its filtered events and its transform | api/route-runtime.spec.ts › TC-RUN-11 | pass | pass |
| TC-RUN-12 | P1 | P3 | Exactly-once, same cluster | exactly_once route | Produce; pause/resume; produce | read_committed consumer sees each event exactly once | api/route-runtime.spec.ts › TC-RUN-12 | pass | pass |
| TC-RUN-13 | P1 | P3 | Replay from a time with duplicates | Running route | Replay without / with confirm | Refused without confirm; done; duplicates delivered | api/route-runtime.spec.ts › TC-RUN-13 | pass | pass |
| TC-RUN-14 | P1 | MVP | Destination key from a field | key mode field | Produce | Output key = field value | api/route-runtime.spec.ts › TC-RUN-14 | pass | pass |
| TC-RUN-15 | P1 | MVP | Destination headers keep selected + add | headers keep_selected | Produce with two headers | Only kept + added headers | api/route-runtime.spec.ts › TC-RUN-15 | pass | pass |
| TC-RUN-16 | P1 | MVP | Start from latest | Events before deploy | Deploy latest; produce | Only new events delivered | api/route-runtime.spec.ts › TC-RUN-16 | pass | pass |
| TC-RUN-17 | P0 | MVP | Destination permanent error stops the route (never silent drop) | Running route | Delete the destination topic / deny ACL | Route Error with message, alert raised | manual (shared broker auto-creates topics) | manual | manual |
| TC-RUN-18 | P0 | MVP | Worker crash recovery by rebalance | Two workers | Kill one worker | Traffic continues within seconds, no loss | manual (local stack has one worker; chaos test) | manual | manual |
| TC-RUN-19 | P1 | MVP | Control plane down: workers keep running | Running route | Stop the API | Events still delivered | manual (stops shared services) | manual | manual |
| TC-RUN-20 | P1 | MVP | Isolation: a stuck route does not stall others | Infinite-loop route + healthy route | Produce | Healthy route delivers promptly | api/route-runtime.spec.ts › TC-RUN-20 | pass | pass |
| TC-RUN-21 | P1 | MVP | Destination transient errors back off without loss | Running route | Pause the broker briefly | No loss, no commit past failure | manual (broker restart on shared cluster) | manual | manual |

## Monitoring: dashboard, metrics, live tail, alerts (UI spec §4, §10, §11.4)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-MON-01 | P0 | MVP | Dashboard health cards count routes | — | Summary before / after a new draft | draft +1; thresholds; metricsAvailable flag | api/monitoring.spec.ts › TC-MON-01 | pass | pass |
| TC-MON-02 | P1 | MVP | Needs attention lists a halted route | Route halted by onError pause | GET /dashboard/attention | Contains the route | api/route-runtime.spec.ts (TC-RUN-08) | pass | pass |
| TC-MON-03 | P1 | MVP | Recent activity: created, deployed, paused | Route deployed + paused | GET /activity | All three verbs for the route | api/monitoring.spec.ts › TC-MON-03 | pass | pass |
| TC-MON-04 | P1 | MVP | Route metrics for 15m / 1h / 24h / 7d | Running route | GET metrics per range; bogus range | 200 with pipeline + series; 4xx for bogus | api/monitoring.spec.ts › TC-MON-04 | pass | pass |
| TC-MON-05 | P1 | P2 | Flow map edge for a deployed route | Running route | GET /flow-map | Edge between its topics | api/monitoring.spec.ts › TC-MON-05 | pass | pass |
| TC-MON-06 | P1 | P3 | First-run checklist | — | GET /onboarding | connection → route → deploy with links | api/monitoring.spec.ts › TC-MON-06 | pass | pass |
| TC-MON-07 | P1 | P3 | Try with demo data | — | POST /onboarding/demo | Demo connection with order-events | api/monitoring.spec.ts › TC-MON-07 | pass | pass |
| TC-MON-08 | P1 | P2 | Alert rule CRUD; webhook write-only | Route | Create, list, patch, delete | secretSet, URL never returned, kept on patch; 204 | api/monitoring.spec.ts › TC-MON-08 | pass | pass |
| TC-MON-09 | P1 | P2 | Alert rule validation | Route | ftp:// webhook; window 0 | 422 both | api/monitoring.spec.ts › TC-MON-09 | pass | pass |
| TC-MON-10 | P1 | P2 | Alert fires into the bell and Needs attention | throughput_zero rule on a running idle route | Wait for evaluation | Alert firing toast and bell entry | manual (local Prometheus scrapes staging, not local workers; covered by web/e2e monitoring-p2 on staging) | manual | manual |
| TC-MON-11 | P1 | P3 | UI: live tail streams input / output | Running route | Route → Live tail; produce | Rows appear; input shows the source topic | ui/studio-monitoring.ui.spec.ts › TC-MON-11 | pass | pass |
| TC-MON-12 | P0 | MVP | UI: dashboard cards and + New route | — | Open / | Four cards, Needs attention, Recent activity, + New route | ui/studio-monitoring.ui.spec.ts › TC-MON-12 | pass | pass |
| TC-MON-13 | P1 | MVP | Overview pipeline numbers (read → passed → sent) match traffic | Route with traffic, metrics scraped | Overview tab | Counts match produced / delivered | manual (local Prometheus scrapes staging only) | manual | manual |
| TC-MON-14 | P2 | P2 | Test notification per channel | Rule with unreachable webhook | POST alert-rules/{id}/test | Per-channel result with ok=false | api/monitoring.spec.ts › TC-MON-14 | pass | pass |
| TC-MON-15 | P2 | P3 | Live tail keeps masked fields masked for viewers | Masked route + viewer | Open Live tail as viewer | Fields masked | manual | manual | manual |
| TC-MON-16 | P1 | MVP | Runtime: observed state and replica versions | Running route | GET /routes/{id}/runtime | running, version 1, ≥ 1 replica | api/monitoring.spec.ts › TC-MON-16 | pass | pass |
| TC-MON-17 | P2 | P2 | Bell feed with unread count | — | GET /alerts | items + unread count | api/monitoring.spec.ts › TC-MON-17 | pass | pass |
| TC-MON-18 | P2 | MVP | Mobile: dashboard, route status, pause / resume, alerts work | Phone viewport | Use the four screens | Usable at phone width | manual | manual | manual |

## Dead letters (UI spec §10.3)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-DLQ-01 | P0 | MVP | List: time, error type, message, offset | Route with failures | GET /routes/{id}/dlq | Items with failedAt, errorType, message, source, version | api/dead-letters.spec.ts › TC-DLQ-01 | pass | pass |
| TC-DLQ-02 | P0 | MVP | Detail: original event, step, JS stack | One failure | GET dlq/{id} | Original key / value; JS step label; stack trace | api/dead-letters.spec.ts › TC-DLQ-02 | pass | pass |
| TC-DLQ-03 | P0 | MVP | Retry after a fix | Fixed v2 deployed | Retry one | Delivered to destination; hidden from the list | api/dead-letters.spec.ts › TC-DLQ-03 | pass | pass |
| TC-DLQ-04 | P1 | MVP | Retry without a fix | Still failing | Retry | Reported failed; still listed | api/dead-letters.spec.ts › TC-DLQ-04 | pass | pass |
| TC-DLQ-05 | P1 | MVP | Discard and restore | Two failures | Discard one; restore it | Hidden; visible again | api/dead-letters.spec.ts › TC-DLQ-05 | pass | pass |
| TC-DLQ-06 | P1 | MVP | Export as JSON | Two failures | GET export?ids= | Attachment, array of 2 details | api/dead-letters.spec.ts › TC-DLQ-06 | pass | pass |
| TC-DLQ-07 | P1 | MVP | Open in Transform Studio | One failure | POST dlq/{id}/studio | Builder URL step=transform; session holds the failing event | api/dead-letters.spec.ts › TC-DLQ-07 | pass | pass |
| TC-DLQ-08 | P1 | MVP | Summary by error type | Three failures | GET dlq/summary | unresolved 3; byErrorType sums to 3 | api/dead-letters.spec.ts › TC-DLQ-08 | pass | pass |
| TC-DLQ-09 | P2 | P2 | Background job retries a filter | Fixed v2; three failures | POST dlq/jobs retry | Job succeeded; list empty; all delivered | api/dead-letters.spec.ts › TC-DLQ-09 | pass | pass |
| TC-DLQ-10 | P1 | MVP | Global dead-letter view | Failure on a route | GET /dlq | Contains the route's events | api/dead-letters.spec.ts › TC-DLQ-10 | pass | pass |
| TC-DLQ-11 | P1 | MVP | UI: list, original event, Retry report | One failure | Dead letters → row → Retry | Original event shown; Retry report dialog | ui/studio-monitoring.ui.spec.ts › TC-DLQ-11 | pass | pass |
| TC-DLQ-12 | P2 | MVP | Filter by text | Two failures | GET dlq?q= | Only matching messages | api/dead-letters.spec.ts › TC-DLQ-12 | pass | pass |

## Dead-letter workflow (dead-letters 00001-workflow)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-DLW-01 | P0 | P4 | Error class on every dead letter | JS step throws | List + detail | errorType step_error, errorClass data on row and detail | api/dead-letters-workflow.spec.ts › TC-DLW-01 | pass | not run |
| TC-DLW-02 | P0 | P4 | Cause groups with normalized pattern | 3 failures, same cause, different ids | GET dlq/groups; list ?fingerprint= | 1 group, count 3, pattern with <n>, byClass data 3; drill-down lists 3 | api/dead-letters-workflow.spec.ts › TC-DLW-02 | pass | not run |
| TC-DLW-03 | P1 | P4 | Global cause view | Failures on a route | GET /dlq/groups?routeId= | Group of the route listed | api/dead-letters-workflow.spec.ts › TC-DLW-03 | pass | not run |
| TC-DLW-04 | P0 | P4 | Dry run produces nothing | One failure | replay dryRun with edit | would_send, output on destination topic, diff /value/bad; nothing produced; still unresolved | api/dead-letters-workflow.spec.ts › TC-DLW-04 | pass | not run |
| TC-DLW-05 | P1 | P4 | Unedited dry run fails again | One failure | replay dryRun | failed with the same message | api/dead-letters-workflow.spec.ts › TC-DLW-05 | pass | not run |
| TC-DLW-06 | P0 | P4 | Edit then replay; audited diff without values | One failure | replay with edit + note | Edited event delivered, resolved; audit dlq.edited_replayed has paths, no values | api/dead-letters-workflow.spec.ts › TC-DLW-06 | pass | not run |
| TC-DLW-07 | P1 | P4 | Invalid edit for a JSON source | One failure | replay with non-JSON value | 400 malformed body pointing at /edit/value (422 is reserved for Avro/Protobuf) | api/dead-letters-workflow.spec.ts › TC-DLW-07 | pass | not run |
| TC-DLW-08 | P1 | P4 | Group dry run samples the group | 3 failures, one group | POST dlq/dry-run {match.fingerprint} | sampled 3, total 3, counts.failed 3 | api/dead-letters-workflow.spec.ts › TC-DLW-08 | pass | not run |
| TC-DLW-09 | P0 | P4 | Bulk replay of a group as a rate-limited job | 4 failures, fixed version | POST dlq/jobs {match.fingerprint, ratePerSecond 2} | Job succeeded 4/4; 4 events delivered | api/dead-letters-workflow.spec.ts › TC-DLW-09 | pass | not run |
| TC-DLW-10 | P1 | P4 | Viewers cannot dry-run or replay | Viewer user | replay / dry-run as viewer | 403 | api/dead-letters-workflow.spec.ts › TC-DLW-10 | pass | not run |
| TC-DLW-11 | P0 | P4 | Strict 4-eyes replay into production | prod connection, editor + approver | sync retry; replay as editor; approve as editor, then approver | 409; 202 pending_approval; self-approve refused; approver approves → edited event delivered | api/dead-letters-workflow.spec.ts › TC-DLW-11 | pass | not run |
| TC-DLW-12 | P1 | P4 | Router-caused DLQ alert rule provisioned by default | New route | GET alert-rules?routeId= | One enabled router_caused_dead_letters rule | api/dead-letters-workflow.spec.ts › TC-DLW-12 | pass | not run |
| TC-DLW-13 | P0 | P4 | Auto-retry heals transient lookup failures; exhausted retries park | HTTP lookup source, retry maxAttempts 2 | CRM fails once; then full outage | First event delivered after a retry; second dead-lettered lookup_unavailable, class transient, attempts 2 | api/dead-letters-workflow.spec.ts › TC-DLW-13 | pass | not run |
| TC-DLW-14 | P1 | P4 | UI: Causes view with class badge | 3 failures | Dead letters → Causes | Bad data badge, pattern "bad event <n>" | ui/workflow-lookups.ui.spec.ts › TC-DLW-14 | pass | not run |

## Lookup sources and lookup steps (connections 00001-lookup-sources, pipeline-engine 00002-lookups)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-LKS-01 | P0 | P4 | Create HTTP source; token write-only; audited | Fake CRM on the runner | POST lookup-sources; GET; audit | 201; secretFields has token, never returned; lookup_source.created audited without it | api/lookup-sources.spec.ts › TC-LKS-01 | pass | not run |
| TC-LKS-02 | P1 | P4 | Invalid settings name the field | — | POST with ftp:// base URL | 422 with /http/baseUrl pointer | api/lookup-sources.spec.ts › TC-LKS-02 | pass | not run |
| TC-LKS-03 | P0 | P4 | Test checklist; auth failure explained | Saved sources (right / wrong token) | POST {id}/test | reachable → authenticated → sample passed, health stored; wrong token fails at authenticated with a reason | api/lookup-sources.spec.ts › TC-LKS-03 | pass | not run |
| TC-LKS-04 | P1 | P4 | Test an unsaved draft | — | POST lookup-sources/test | ok; nothing stored | api/lookup-sources.spec.ts › TC-LKS-04 | pass | not run |
| TC-LKS-05 | P1 | P4 | PATCH needs If-Match and keeps secrets | Saved source | PATCH without / with If-Match | 428; 200, token kept and still authenticates | api/lookup-sources.spec.ts › TC-LKS-05 | pass | not run |
| TC-LKS-06 | P0 | P4 | Delete blocked while used; unknown sourceId invalid | Route uses the source | DELETE; validate route with unknown sourceId | 409 + usedBy lists the route; validation reports sourceId | api/lookup-sources.spec.ts › TC-LKS-06 | pass | not run |
| TC-LKS-07 | P0 | P4 | Preview calls the live source | Sample session | POST preview/run | Enriched output (tier gold); CRM called | api/lookup-sources.spec.ts › TC-LKS-07 | pass | not run |
| TC-LKS-08 | P0 | P4 | Running route enriches; outage → lookup_unavailable | Running route with lookup step | Produce 2; outage; produce 1 | Outputs enriched; dead letter lookup_unavailable, class transient, step crm | api/lookup-sources.spec.ts › TC-LKS-08 | pass | not run |
| TC-LKS-09 | P1 | P4 | UI: Lookup sources tab | Saved source | Connections → Lookup sources, search | Row with name, HTTP kind and target | ui/workflow-lookups.ui.spec.ts › TC-LKS-09 | pass | not run |

## Audit log (UI spec §11.3)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-AUD-01 | P0 | P3 | Every mutation audited with actor, time, IP | — | Create connection; query audit | connection.created with actor, at, ip | api/audit-settings-security.spec.ts › TC-AUD-01 | pass | pass |
| TC-AUD-02 | P1 | P3 | Filter by object and action | Route paused | GET ?objectId=&action=route.paused | Exactly one entry labelled with the route | api/audit-settings-security.spec.ts › TC-AUD-02 | pass | pass |
| TC-AUD-03 | P1 | P3 | Raw sample views are sensitive entries | Sensitive connection | Load samples as admin; query sensitive=true | Entry present | api/audit-settings-security.spec.ts › TC-AUD-03 | pass | pass |
| TC-AUD-04 | P1 | P3 | Only workspace admins read the audit log | Editor | GET /audit-log | 403 | api/audit-settings-security.spec.ts › TC-AUD-04 | pass | skip |
| TC-AUD-05 | P1 | P3 | Read-only | — | PATCH / PUT / DELETE / POST | All refused | api/audit-settings-security.spec.ts › TC-AUD-05 | pass | pass |
| TC-AUD-06 | P2 | P3 | Export JSONL, itself audited | — | GET export?format=jsonl | Lines include the entry; audit.exported recorded | api/audit-settings-security.spec.ts › TC-AUD-06 | pass | pass |
| TC-AUD-07 | P1 | P3 | No secrets in audit diffs | — | see TC-SEC-01 | — | api/audit-settings-security.spec.ts (TC-SEC-01) | pass | pass |
| TC-AUD-08 | P1 | P3 | UI: filter and open an entry | Connection created | Audit log → filter → open row | Drawer shows the entry | ui/studio-monitoring.ui.spec.ts › TC-AUD-08 | pass | pass |

## Settings

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-SET-01 | P1 | P3 | Workspace settings readable by members | Viewer | GET /workspace-settings | 200 with aiAssistEnabled | api/audit-settings-security.spec.ts › TC-SET-01 | pass | skip |
| TC-SET-02 | P1 | P3 | Changing settings needs settings.manage; audited | Admin, editor | PATCH as admin, as editor | settings.updated entry; editor 403 | api/audit-settings-security.spec.ts › TC-SET-02 | pass | skip |
| TC-SET-03 | P1 | P2 | Teams: create, rename, delete blocked when not empty | — | Create (prod approval default on), rename, add a team-owned connection, delete; remove the connection, delete | 201, 200, 409; then 204 (empty team is deleted) | api/audit-settings-security.spec.ts › TC-SET-03 | pass | pass |
| TC-SET-04 | P2 | P2 | Pre-invite a user | — | POST /users/invitations | 201; user listed as invited | api/audit-settings-security.spec.ts › TC-SET-04 | pass | pass |
| TC-SET-05 | P1 | MVP | API token secret shown once | — | Create; list | Secret ert_… once; list shows prefix only | api/audit-settings-security.spec.ts › TC-SET-05 | pass | pass |
| TC-SET-06 | P1 | MVP | UI: members & roles and API tokens | — | Open Settings | Members & roles section; New token button | ui/studio-monitoring.ui.spec.ts › TC-SET-06 | pass | pass |
| TC-SET-07 | P2 | MVP | Dark and light themes | — | Toggle theme | Both themes readable, status colours consistent | manual (visual) | manual | manual |

## Security: secrets, sandbox, authentication (SysDesign §7.5, §11)

| ID | Pri | Phase | Title | Preconditions | Steps | Expected | Automated by | Local | Staging |
|---|---|---|---|---|---|---|---|---|---|
| TC-SEC-01 | P0 | MVP | Connection password never returned | SCRAM connection with password | GET, list, route export, audit | Password absent everywhere; secretFields set | api/audit-settings-security.spec.ts › TC-SEC-01 | pass | pass |
| TC-SEC-02 | P0 | MVP | Payloads never in worker logs | Local stack | Produce marked payloads incl. a failing one | Marker absent from the worker log | api/audit-settings-security.spec.ts › TC-SEC-02 | pass | skip |
| TC-SEC-03 | P0 | MVP | Sandbox: no network / require / process | — | typeof fetch, XMLHttpRequest, require, process, WebSocket | All undefined | api/audit-settings-security.spec.ts › TC-SEC-03 | pass | pass |
| TC-SEC-04 | P0 | MVP | Sandbox: infinite loop hits the time limit | — | Preview `while(true){}` | error (timeout) quickly | api/audit-settings-security.spec.ts › TC-SEC-04 | pass | pass |
| TC-SEC-05 | P0 | MVP | Sandbox: memory bomb hits the memory limit | — | Preview allocating loop | error (memory / limit) | api/audit-settings-security.spec.ts › TC-SEC-05 | pass | pass |
| TC-SEC-06 | P1 | MVP | No host globals | — | List suspicious globalThis names | None | api/audit-settings-security.spec.ts › TC-SEC-06 | pass | pass |
| TC-SEC-07 | P1 | MVP | Check API: syntax error line / column | — | POST /sandbox/check | Diagnostic on line 2; none for valid code | api/audit-settings-security.spec.ts › TC-SEC-07 | pass | pass |
| TC-SEC-08 | P1 | MVP | No state leaks between events | — | Counter on globalThis + prototype pollution | Fresh context per event (1,1,1) | api/audit-settings-security.spec.ts › TC-SEC-08 | pass | pass |
| TC-SEC-09 | P0 | MVP | Unauthenticated calls get 401 | No session | GET routes, connections, me, audit | 401 | api/audit-settings-security.spec.ts › TC-SEC-09 | pass | pass |
| TC-SEC-10 | P0 | MVP | CSRF required on cookie writes | Session cookie, no header | POST /connections | 403 | api/audit-settings-security.spec.ts › TC-SEC-10 | pass | pass |
| TC-SEC-11 | P1 | MVP | Dev login refused outside local | Staging | POST /auth/dev/login | Refused | api/audit-settings-security.spec.ts (staging only) | skip | pass |
| TC-SEC-12 | P1 | MVP | Secrets stored only as ciphertext | DB access | Inspect connections + secrets tables | No plaintext password | manual (needs DB access) | manual | manual |
| TC-SEC-13 | P1 | MVP | Workers fetch secrets via the internal endpoint only | — | Call /internal/v1 without workload identity | Refused | manual (internal listener not exposed through the web app) | manual | manual |
| TC-SEC-14 | P1 | MVP | Route JS limits clamped to the maxima | runtime-info maxima | Preview infinite loop with jsTimeoutMs 5000 | Defaults 50 ms / 64 MB published; error after ≤ maximum (1 s), not 5 s | api/audit-settings-security.spec.ts › TC-SEC-14 | pass | pass |
| TC-SEC-15 | P2 | MVP | Maximum output size per JS call | — | Return a > 1 MB value | error output too large | manual | manual | manual |

## Known failures

Filled in from the last run (see `report/html` for traces).

<!-- known-failures:start -->
| Target | ID | Reason | Error of the last run |
|---|---|---|---|
| staging | TC-TS-35 | Product limitation, not fixed: the embedded QuickJS build records source positions only at calls/throws, so a TypeError from a property access (`event.value.order.items` on undefined) is reported at the function start (1:1) instead of line 2. Thrown errors are positioned correctly (TC-TS-14). Fixing needs a QuickJS/WASM rebuild with per-opcode line info (pkg/sandbox/quickjs). | Error: QuickJS reports the function start for property-access TypeErrors |
| local | TC-TS-35 | Product limitation, not fixed: the embedded QuickJS build records source positions only at calls/throws, so a TypeError from a property access (`event.value.order.items` on undefined) is reported at the function start (1:1) instead of line 2. Thrown errors are positioned correctly (TC-TS-14). Fixing needs a QuickJS/WASM rebuild with per-opcode line info (pkg/sandbox/quickjs). | Error: QuickJS reports the function start for property-access TypeErrors |
<!-- known-failures:end -->
