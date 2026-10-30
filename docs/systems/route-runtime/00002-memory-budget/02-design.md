# Route Runtime — Design (00002-memory-budget)

Incremental version on top of `00001-stress-test`. **Unchanged:** sections 1–11 of `00000-init/02-design.md` and
the stress-test design, lane marks (RR-ST-30) and heartbeat fix (RR-ST-31) of `00001-stress-test/02-design.md`.
This version adds one worker-wide memory budget on top of the per-partition lane backpressure of flow 5.1.

## 1. Pieces

| Piece | Where | Role |
|---|---|---|
| `membudget.Budget` | `internal/runtime/membudget/budget.go` | Global used bytes (atomic), open accounts, paused set, fair share, watermarks |
| `membudget.Account` | `account.go` | One per runner: `Charge` / `Release` / `Close`; applies pause/resume to its `Pauser` |
| `membudget.Pauser` (base) | `budget.go` | `PauseFetch` / `ResumeFetch`; implemented by `runner.SourcePauser` |
| `RecordCost` | `account.go` | key + value + header bytes + `RecordOverheadBytes` (512) |
| `CgroupLimits`, `Resolver`, `RuntimeMemLimit` | `limits.go` | cgroup v2/v1 limit, GOMEMLIMIT read/apply, budget sizing |
| `SettingsLoader` | `settings.go` | `ROUTER_WORKER_MEMORY_BUDGET_BYTES`, `_PERCENT` (60), `ROUTER_WORKER_GOMEMLIMIT_PERCENT` (90) |
| `Builder` | `builder.go` | Settings → resolution → `Budget` + gauges; logs the sizing once at start-up |
| `runner.MemoryAccount`, `SourcePauser` | `internal/runtime/runner/memory_budget.go` | Lane view of the account; topic-level pause over the source client |

Thresholds (`membudget.Watermark`, percent): high = 100 % of the budget, low = 80 %, resume share = 50 % of the
fair share. Fair share = budget / open accounts.

## 2. Sizing at worker start

```
worker Assembly.runnerDeps [start-up]
  └─ membudget.NewBuilder(log).Build(registry, workerID)
      └─ SettingsLoader.Load → invalid value → worker fails to start (error names the key)
      └─ Resolver.Resolve
          └─ cgroup limit (v2 memory.max, else v1 memory.limit_in_bytes; "max" / ≥ 2^60 = none)
          └─ GOMEMLIMIT env unset and cgroup limit found → debug.SetMemoryLimit(90 % × cgroup) → GOMEMLIMIT applied
          └─ memory limit = operator GOMEMLIMIT → else cgroup → else 1 GiB fallback
          └─ budget = BUDGET_BYTES if > 0 → else 60 % × memory limit
      └─ register gauges (scrape-time GaugeFuncs) → log budget_bytes, memory_limit_bytes, limit_source, gomemlimit
  └─ runner.Deps.Budget = budget (nil in tests/tools → unlimited)
```

Example: container limit 1 GiB → GOMEMLIMIT 921 MiB, budget 614 MiB.

## 3. Accounting (changes flow 5.1)

```
poll loop [per runner] → cycle.Run → dispatch → lane.Enqueue(recs)
  └─ RecordDetach: copy key / value / header values out of the fetch response (integration fix)
  └─ account.Charge(RecordsCost(recs))                        (lane queue marks unchanged)
      └─ this charge crosses the high watermark → pause every account with used ≥ share
      └─ else this route used ≥ share and worker used ≥ high → pause this account
lane goroutine → next() → RecordFlow.Handle(rec)
  └─ route halted / limiter ctx ended → lane.Discard(rec) → account.Release(cost)
  └─ else ticket.cost = RecordCost(rec) → process → deliver
ticket finish / abandon [async: ack callback, retry goroutine, halt]
  └─ release → lane.Settle(cost) → account.Release(cost)
      └─ any route paused → resumeEligible: each paused route with worker used ≤ low or own used < 50 % share → resume
lane.Stop (revoke, lost, stop, aborted EOS batch)
  └─ account.Release(cost of queued records)                  (they were not tracked; re-read by the next owner)
RouteRunner.Stop
  └─ … drain, commit, close consumer → account.Close
      └─ return remaining usage → shares grow → resumeEligible for the other routes
```

So the budget counts queued plus in-flight records of every runner. A route under its share is never paused,
so worker usage is bounded by budget + at most one poll per route (≤ `MaxPollRecords` records) arriving between
the decision and the pause taking effect.

## 4. Pause / resume mechanics

```
pause decision [poll goroutine, under budget lock]
  └─ held set += account, paused gauge +1, pauses counter +1, desired = paused
  └─ after unlock: account.sync() → SourcePauser.PauseFetch() → client.PauseFetchTopics(source topics)
      └─ next PollRecords: franz-go strips buffered data of paused topics; cursors stay at the first unpolled
         offset → stripped records are fetched again after resume (no loss, no reorder)
resume decision [any goroutine, under budget lock]
  └─ held set −= account, paused gauge −1, desired = running → signal account goroutine [async]
      └─ account.sync() → SourcePauser.ResumeFetch() → client.ResumeFetchTopics(paused topics)
```

- `sync` is serialised per account and applies the *latest* desired state, so a pause racing a resume always
  ends in the last decision.
- Pauses run synchronously on the poll goroutine so the very next poll is paused (measured: peak usage ≈ one
  poll instead of the ~2 MiB the lane caps alone allow in the test). Resumes are asynchronous because releases
  happen inside produce callbacks and rebalance callbacks, where calling back into the client must be avoided.
- Topic-level pauses (`PauseFetchTopics`) are independent of the gate's partition pauses (`queue_full`,
  `backoff`): franz-go keeps both sets, and resuming one never lifts the other. Pausing whole topics also covers
  partitions assigned while the route is paused. Pattern sources pause the topics `GetConsumeTopics` returns.
- Commits are untouched: records already handed to lanes are processed and committed normally; the
  exactly-once batch barrier is unaffected (a pause only stops the next fetch).

## 5. Metrics

| Metric | Type | Labels | Meaning |
|---|---|---|---|
| `router_worker_memory_budget_bytes` | gauge | `worker` | Budget size |
| `router_worker_memory_used_bytes` | gauge | `worker` | Bytes held in lanes + in flight |
| `router_worker_memory_paused_routes` | gauge | `worker` | Routes currently paused by the budget |
| `router_worker_memory_pauses_total` | counter | `worker` | Budget pauses since start |

All are read at scrape time; the record path only does atomic adds. `ReplicaStatus.MemBytes` is unchanged
(lane queue bytes).

## 6. Decisions

- D-MB-1 Count raw record bytes + fixed overhead, not decoded size: cheap and deterministic; the 40 % headroom
  below the memory limit (and GOMEMLIMIT at 90 %) absorbs decode/encode amplification, JS heaps and client buffers.
- D-MB-2 Fair share instead of global stop: one hot or stuck route cannot starve the others.
- D-MB-3 Topic pause instead of per-partition pause: covers late assignments and does not interfere with the
  gate's reference-counted partition holds.
- D-MB-4 Never set GOMEMLIMIT when the operator set it; never override an explicit `_BUDGET_BYTES`.
- D-MB-5 (wave-2 integration) Records are detached from their fetch response on enqueue. franz-go records alias
  the whole fetch response (all partitions of one fetch); in the 25-route overload one slow lane's records pinned
  whole responses: heap 3–4 GB of which 92 % `brokerCxn.readSizeAndBody`, while the budget charged ~300 MB. One
  copy per record (≈ 1 KB memcpy) makes the charge the real retained size.
- D-MB-6 (wave-2 integration) Source consumer fetches are sized from the budget (`SourceFetch`,
  `Budget.FetchLimits`: 1 % of the budget within [1 MiB, 50 MiB], partition max ≤ 1 MiB); fetched-but-not-polled
  data is outside the budget. An explicit `fetch.max.bytes` connection property wins.
- D-MB-7 (wave-2 integration) A lane clears the queue slot of every record it hands out: `queue = queue[1:]` alone kept
  settled records reachable through the backing array until the next reallocation.

## 7. Tests

- `membudget/budget_test.go`: pause only when the worker is over budget and the route over its share; hysteresis
  (stay paused above 80 %, resume at 80 %); resume when a route leaves; single route; unlimited; closed account
  ignores updates; record cost; gauge registration.
- `membudget/limits_test.go`: cgroup v2/v1/unlimited/none, GOMEMLIMIT applied from cgroup, operator GOMEMLIMIT
  kept, fallback, configured bytes, settings validation.
- `runner/runner_budget_test.go` (kfake): 6000-record backlog vs a 64 KiB budget — ≥ 2 pause cycles, peak usage
  ≤ budget + one poll, every record delivered exactly once and in order per key, usage 0 after stop.
- `runner/memory_budget_test.go`: `SourcePauser` (configured and pattern topics), lane charge/release balance.
