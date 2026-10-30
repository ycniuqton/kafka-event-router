# Dead letters — Plan (00001-workflow)

Rules: read this file first; mark the step you start `[~]`, finished `[x]`; never delete items (add an inline note if skipped).
Carried over from 00000-init: nothing open (every item there is `[x]`). Design refs are to this folder's `02-design.md`.

## Docs
- [x] DLW-00 Version folder 00001-workflow (requirements, design, plan)

## BE — contracts
- [x] DLW-01 `internal/dlq/record`: new ErrorTypes, `ErrorClass` + `ErrorClasses` + `Class()` + `TypesOf`, header `router.retry_attempts` / `Failure.Attempts` in Codec (§2)
- [x] DLW-02 `pkg/engine/dependency.go`: `DependencyKind`, `DependencyError`, `AsDependencyError` — transient contract for F9 lookups (§2.1)
- [x] DLW-03 Unit tests: classification table (every type has a class), codec attempts round trip, dependency error unwrap

## BE — cause grouping
- [x] DLW-10 `internal/dlq/fingerprint.go`: `MessageNormalizer` (ordered rules), `Fingerprinter`, `Fingerprint` (§3.1) + table tests
- [x] DLW-11 `internal/dlq/groups.go`: `GroupQuery`, `GroupSet`, `TrendBuckets`, `GroupPage`, `GroupCache` + tests
- [x] DLW-12 `Manager.Groups` / `Manager.GlobalGroups` (bounded scans, warnings) (§3.2)
- [x] DLW-13 `Query.Fingerprint` / `Query.Classes` filters + list/detail/summary DTO fields `errorClass`, `fingerprint`, `attempts`, `byErrorClass`
- [x] DLW-14 HTTP: `GroupsHandler`, `GlobalGroupsHandler`, list/match query parsing of `fingerprint` / `class`

## BE — edit, dry-run, replay
- [x] DLW-20 `internal/dlq/edit.go`: `RecordEdit`, `EditValidator`, `MaskMerger`, `StructuralDiff`, `EditPlanner` + tests
- [x] DLW-21 `DryRunner` on `InProcessExecutor` (no produce) and `PreviewExecutor` (preview run) + `DryRunResult` views
- [x] DLW-22 `Manager.Replay` (single, edited or not; dry-run; gate; audit `dlq.edited_replayed`) + `Manager.DryRun` (selection ≤ 20)
- [x] DLW-23 `ReplayGate` port + `AlwaysAllowGate`; sync retry → 409 `replay_approval_required` when the gate requires approval
- [x] DLW-24 HTTP: `ReplayHandler`, `DryRunHandler` + handler tests

## BE — jobs (bulk replay, rate limit, approval)
- [x] DLW-30 Migration `3100_dlq_workflow` (job statuses, rate, edit, note, decision, alert kind, provisions)
- [x] DLW-31 jobs: ids selection, `ratePerSecond` token bucket, pending_approval/rejected, edited jobs, approve/reject in Service + Memory/Postgres repositories
- [x] DLW-32 HTTP: create job body (ids, rate, note, match fingerprint/class), `ApproveJobHandler`, `RejectJobHandler`, pending list + tests
- [x] DLW-33 Wiring `cmd/api/wire_dlq.go`: approvals gate adapter, new audit actions in catalog

## BE — alerts
- [x] DLW-40 Kind `router_caused_dead_letters` (limits, evaluator, PromQL catalog entry) + tests
- [x] DLW-41 `DefaultRuleProvisioner` + pgstore `ProvisionMissing`; workspace setting `router_caused_alert_default` (default on)

## BE — auto-retry (route-runtime)
- [x] DLW-50 Schema `errorPolicy.retry` + `routeconfig_gen.go` + validator bounds (maxDelay ≥ base) + TS `routeConfig.ts`
- [x] DLW-51 `failures.DeadLetterTypes` (panic, sandbox crash, config, dependency, encode) + `CategoryDependencyTransient` + `PanicError`
- [x] DLW-52 `internal/runtime/retrytopic`: `BackoffSchedule`, `TierNaming`, `Scheduler`, header codec + unit tests
- [x] DLW-53 Retry `Consumer` (tiers, due-time pause, reprocess, next tier / park with attempts) wired into the runner — retrytopic.Consumer + runner RetryHandler; kfake tests recover + park-with-attempts
- [x] DLW-54 Retry metrics (scheduled / recovered / exhausted, pending gauge)

## FE
- [x] DLW-60 Domain + contracts + mock + HTTP: error class, groups, edit/replay, dry-run, job rate/approval, alert kind, retry policy types
- [x] DLW-61 Causes view (`CauseGroupList`, `ErrorClassBadge`, class chips) on the global screen and route tab
- [x] DLW-62 `RecordEditor` + `ReplayDialog` (dry-run preview, diff, approval state) in the detail panel
- [x] DLW-63 Bulk replay of a group / selection with rate + progress; approvals panel (approve/reject)
- [x] DLW-64 Builder `RetryPolicyFields`; alert rule kind option; workspace default toggle — retry fields live in route detail → Settings → Error policy (`RetryPolicyFields`), the route settings surface that edits `errorPolicy`
- [x] DLW-65 Vitest: grouping/class models, edit validation, replay dialog state, retry policy form — 5 new files (34 tests); pre-existing HttpServicesFactory.test failure (window undefined in push module) untouched

## Verify
- [x] DLW-70 `go test ./...` (no integration env), `go vet`, web build + oxlint + vitest — all Go packages pass; web build + oxlint clean; vitest 381/383 (2 pre-existing HttpServicesFactory failures, files untouched)
- [x] DLW-71 OpenAPI paths/schemas for the new endpoints (`api/openapi/paths/dead-letters.yaml`)
- [x] DLW-72 Acceptance cases listed for test/acceptance/TEST-CASES.md (handed to the integration step) — listed in the F8 hand-off for the integration step (not run here) — integration: automated as TC-DLW-01..14 (`test/acceptance/specs/api/dead-letters-workflow.spec.ts`, `specs/ui/workflow-lookups.ui.spec.ts`), all pass on local

## Integration
- [x] DLW-80 Lookup contract unified with pipeline-engine 00002-lookups (PE-LK-33): lookup steps wrap transient causes in `DependencyError{Kind: lookup}` with engine type `lookup_unavailable`; DLQ type `lookup_unavailable` (transient), `lookup_miss` → `step_error` (data); `failures/lookup_test.go`
- [x] DLW-81 DLQ dry-run/retry engine runs serve lookup steps through the API's in-process gateway (`AssemblyDeps.Sources`, `PipelineCache.WithSources`); `internal/dlq/pipeline_sources_test.go` (hit + dry-run `lookup_unavailable`)
- [x] DLW-82 Migration renumbered `2100_dlq_workflow` → `3100_dlq_workflow` (P4 range, dead-letters block; 2100 was in the P3 range)
