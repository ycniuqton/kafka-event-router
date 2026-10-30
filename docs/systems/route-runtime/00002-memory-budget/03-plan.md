# Route Runtime — Plan (00002-memory-budget)

Incremental version on top of `00001-stress-test`. **Unchanged:** every item of `00000-init/03-plan.md` and
`00001-stress-test/03-plan.md` keeps its state there (RR-OPT-03 is implemented by this plan). Mark the step you
start `[~]`, finished `[x]`; never delete items.

## BE
- [x] RR-MB-01 `membudget.Budget` (global atomics, fair share, hysteresis, paused set) + `Account` per runner + `Pauser` base — `internal/runtime/membudget/budget.go`, `account.go`, `builder.go`
- [x] RR-MB-02 Memory limit detection (GOMEMLIMIT, cgroup v2/v1) + `ApplyGoMemLimit` + settings loader (`ROUTER_WORKER_MEMORY_BUDGET_*`, `ROUTER_WORKER_GOMEMLIMIT_PERCENT`) — `internal/runtime/membudget/limits.go`, `settings.go`
- [x] RR-MB-03 Metrics gauges `router_worker_memory_budget_bytes` / `_used_bytes` / `_paused_routes` — `internal/runtime/membudget/metrics.go`
- [x] RR-MB-04 Runner integration: account opened in `launch`, charged on lane enqueue, released on finish/abandon/lane stop, closed on stop; topic pauser over the source client — `internal/runtime/runner/{runner,runner_lifecycle,partition_lane,record_flow}.go` (small edits; shared with F5)
- [x] RR-MB-05 Worker wiring: apply GOMEMLIMIT, build the budget, pass it in `runner.Deps` — `cmd/worker/assembly.go`
- [x] RR-MB-06 Unit tests: budget decisions, limits/settings, kfake runner test (pause → resume, no loss, order kept) — `internal/runtime/membudget/*_test.go`, `internal/runtime/runner/runner_budget_test.go`
- [x] RR-MB-07 Docs: design (this folder)
- [x] RR-MB-10 Integration fix: detach polled records from the fetch response (`membudget.RecordDetach`, `PartitionLane.Enqueue`) — heap profile of the 25-route overload showed 92 % fetch buffers pinned by queued records; test `detach_test.go`
- [x] RR-MB-11 Integration fix: size source fetches from the budget (`runner.SourceFetch`, `Budget.FetchLimits`); tests `TestBudgetFetchLimitsScaleWithTheBudget`, `TestSourceFetchSizesFromTheBudgetUnlessTheConnectionSetsIt`
- [x] RR-MB-12 Integration fix: lane `next()` clears the handed-out slot so the queue backing array does not keep settled records alive (profile: detached records 452 MB vs 280 MB charged at 50 routes); test `TestLaneDoesNotKeepHandedOutRecordsAlive`; fetch share lowered to 1 % of the budget
- [x] RR-MB-09 Integration: `Budget.UnderPressure()` feeds the adaptive in-flight limiter (route-runtime 00003-autotune §4, RR-AT-42)

## Verification (orchestrator, after merge)
- [x] RR-MB-08 Re-run S3 (many routes) and an overload case (slow destination, worker memory limit e.g. 512 MiB): RSS stays under the limit, no OOM kill, lost = 0, paused-routes gauge > 0 during overload — done (wave 2): S3 1000 routes pass (p99 307 ms, RSS 434 MB); overload (GOMEMLIMIT 512 MiB, budget 256 MiB, 50 fan-out routes, output side saturated): lost 0, 50 routes paused, no crash; RSS 580 MB after RR-MB-10..12 (was 2.2–4.2 GB; wave 1 16 GB) — still ~13 % over 512 MiB, see open issue in the wave-2 report; no cgroup available locally, so a hard OOM kill was not tested
