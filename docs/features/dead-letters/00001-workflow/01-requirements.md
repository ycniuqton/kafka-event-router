# Dead letters — Requirements (00001-workflow)

Incremental on 00000-init. **Unchanged:** every FR/NFR of 00000-init (browse, detail, retry, discard, restore,
export, studio, jobs, masking, audit, demo, exactly-once). This version adds the dead-letter *workflow*.

## 1. Business intent
After a manual review of all DLQ records the team had to group them by hand to tell "our bug" from "bad data".
The workflow answers three questions without spreadsheets: *what distinct causes are there*, *whose fault is each*,
and *how do I fix and replay the affected events safely* — plus transient failures should heal on their own.

## 2. Functional requirements

### Cause grouping
- FR-W1 Fingerprint = route + step + error type + normalized message (ids, uuids, hex, numbers, offsets, positions,
  timestamps replaced by placeholders). Same cause ⇒ same fingerprint, across versions and partitions.
- FR-W2 Groups list per route and globally: pattern, error type, class, step, count, first/last seen, sample record,
  versions seen, hourly trend (24 buckets); sorted by count; filters time range, error type, class, route.
- FR-W3 A group drills into the regular list (`fingerprint` filter) and is a valid bulk-action selection.
- FR-W4 Group scans are bounded (records/time) and flagged `approximate`/`truncated`.

### Classification
- FR-W5 Every error type has exactly one class: **data** (bad input: step_error from the user script, decode_error,
  output_invalid/validation), **router** (internal_error, sandbox_crash, encode_error, produce_error, config_error,
  unknown/unreadable), **transient** (timeout, lookup_unavailable, dependency_unavailable, produce_retriable).
- FR-W6 Rows, details, groups and summaries carry `errorClass`; the UI shows a badge (Bad data / Router bug / Transient)
  and class filter chips.
- FR-W7 New alert rule kind `router_caused_dead_letters` (count of router-class dead letters in window > threshold, default 0).
- FR-W8 The rule is provisioned **enabled by default** for every route of the workspace (once per route; a user may
  disable or delete it and it is not re-created); a workspace setting turns the default off.

### Edit-then-replay
- FR-W9 Edit a single record's key, value and headers in a JSON editor, validated against the source key/value format
  (JSON source ⇒ valid JSON); Avro/Protobuf sources are not editable (422).
- FR-W10 Masking is respected: Viewers never edit (no dlq.act); masked editors edit the masked view and every leaf
  left at its mask is restored from the original server side (no raw value ever reaches the browser).
- FR-W11 Dry-run preview of a record (edited or not) or of a selection/group sample (≤ 20) through the route's current
  active version — outputs per branch, filtered or failed with the new error; nothing produced, nothing resolved.
- FR-W12 Replay of an edited record is audited (`dlq.edited_replayed`) with a structural diff (JSON paths + op,
  values never stored) and the note.
- FR-W13 Bulk replay of a group or a selection (ids or match) as a background job with a rate limit
  (events/s, default 50, max 1000) and live progress (processed/total, counts, cancel).

### Governance
- FR-W14 Edit, replay and bulk replay require `dlq.act` (Editor+); dry-run requires `dlq.act`.
- FR-W15 Strict 4-eyes: when the route's destinations include a `prod` connection and the deploy gate would require
  approval (team requires prod approval, or caller lacks route.deploy_prod), any replay becomes a pending job
  (`pending_approval`) that a different person with `deployment.approve` approves or rejects. Sync retry answers
  409 `replay_approval_required`.

### Auto-retry of transient errors
- FR-W16 Per-route `errorPolicy.retry {enabled, maxAttempts, baseDelayMs, maxDelayMs}`: exponential backoff
  delay(n) = min(base·2^(n-1), max), n = 1..maxAttempts (1..10).
- FR-W17 Transient processing failures (timeout after in-memory retries, lookup/dependency unavailable, registry
  outage on encode) go to tiered retry topics without blocking the source partition; when attempts are exhausted
  the record follows `onError` (dlq: parked with its transient type and `router.retry_attempts`).
- FR-W18 Config in schema + Go/TS types + Builder UI (route settings → error handling); counters
  scheduled/recovered/exhausted and a pending gauge; dead letters show the attempt count.

## 3. Non-functional / constraints
- NFR-W1 DLQ payloads stay out of Postgres, with one exception: a pending-approval edited replay stores its edit
  (≤ 1 MiB) in `dlq_jobs.edit`, cleared when the job ends.
- NFR-W2 Fingerprinting/classification are pure, deterministic and unit-tested; no payload in fingerprints.
- NFR-W3 Retry topics: per attempt tier, on the DLQ connection, 7-day retention; retries are at-least-once
  (also for exactly_once routes).
- NFR-W4 Coding rules (.devlens/rules.md) as in 00000-init.

## 4. Out of scope
- Retry topics for produce/destination outages (they keep the blocking partition backoff — every record would fail).
- Editing Avro/Protobuf payloads; replay into another destination; multi-record editing.

## 5. Dependencies
- route-runtime (worker classifier, retry topics), pipeline-engine (schema `errorPolicy.retry`, `engine.DependencyError`),
  monitoring (alerts kinds/evaluator, PromQL catalog), access-control (approvals gate, rbac), lookup steps (F9) produce
  `engine.DependencyError{Kind: lookup}` for unavailable lookups.
