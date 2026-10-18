# Acceptance suite

Acceptance tests derived from the product specification. The catalogue of cases, with the status of the last run, is
[`TEST-CASES.md`](TEST-CASES.md). Every automated test is named after its case id (`TC-<AREA>-<n> …`).

- `specs/api/*.spec.ts` (Playwright project `api`): backend behaviour through the public REST API (Playwright request
  context) plus Kafka checks with kafkajs: delivery, ordering, DLQ, retry, replay, exactly-once, hot reload, rollback,
  RBAC 403s, approvals, secret non-disclosure, sandbox limits, audit.
- `specs/ui/*.spec.ts` (project `ui`): key UI flows in Chromium (connection panel, builder → deploy, Studio, history,
  dashboard, live tail, dead letters, audit log, settings).
- `lib/`: target profile, login strategies, API session (CSRF + If-Match), Kafka client, route-draft builder,
  per-test resource registry with cleanup, sweeper of leftovers, page helpers.

## Run

One-time setup:

```sh
source ~/.nvm/nvm.sh
cd test/acceptance && npm install
npx playwright install chromium     # if not installed yet (see web/e2e/README.md for missing system libraries)
```

Local stack (`deploy/dev/dev.sh up`: web :5173 with dev login, api :8080, worker, preview, tunnel to the staging
Postgres / Redis / Kafka):

```sh
npm run acc:local       # = ACC_TARGET=local playwright test; then writes the results into TEST-CASES.md
```

Staging (Dex OIDC login with `STAGING_LOGIN_EMAIL` / `STAGING_LOGIN_PASSWORD` read from
`~/.secrets/event-router-staging.env`; the dev tunnel must be up for the runner's Kafka access):

```sh
deploy/dev/tunnel.sh start
npm run acc:staging     # = ACC_TARGET=staging playwright test; then updates TEST-CASES.md
```

Useful variants: `npx playwright test --project=api`, `npx playwright test -g TC-RUN-`, `npm run catalogue` (re-write
the catalogue from the last `report/results.json`).

### Targets

| | `ACC_TARGET=local` (default) | `ACC_TARGET=staging` |
|---|---|---|
| Base URL | `http://localhost:5173` | `STAGING_URL` (else `http://$STAGING_HOST`) |
| Login | dev login (`POST /auth/dev/login`); extra users by email | Dex form, single static user |
| Kafka bootstrap used in connections (platform view) | `127.0.0.1:29092` | `kafka:19092` |
| Kafka for the test runner (kafkajs) | `127.0.0.1:29092` (dev tunnel) | `127.0.0.1:29092` (dev tunnel, same cluster) |

Overrides: `ACC_BASE_URL`, `ACC_PLATFORM_BOOTSTRAP`, `ACC_RUNNER_BOOTSTRAP`, `ACC_SECRETS_FILE`, `ACC_PLAYWRIGHT_LIBS`
(folder with chromium's system libraries, default `~/.local/lib/playwright-deps/lib`). Secrets are read one key at a
time and never printed.

On staging, multi-role scenarios (viewer / editor / approver users) are skipped: Dex has one static user and dev login is
refused there (TC-SEC-11 checks that). Local-only checks (worker log scan) are skipped on staging too.

## Isolation and cleanup

Kafka is shared with staging. Everything the suite creates is named `acc-<area>-<id>-…` (connections, topics, routes,
consumer groups `router.acc-…`, DLQ topics `router.dlq.acc-…`, test topics `router.test.acc-…`, teams, tokens, users
`acc-…@local.dev`). Each test registers what it creates and removes it at the end (routes are archived — there is no hard
delete — connections, topics, groups, alert rules, teams deleted; extra users disabled). A global setup/teardown sweeper
removes `acc-` leftovers of interrupted runs, so the suite is re-runnable. Do not run two suites against the same Kafka at
the same time (the sweeper would remove the other run's topics).

## Reading the report

- Console: one line per test (`list` reporter); failures print the assertion with the API response body.
- `report/html/index.html` (`npm run report`): per-test steps, errors, traces (`trace.zip` on failure: open the trace
  viewer from the report) and screenshots of failed UI tests.
- `report/results.json`: machine-readable results; `npm run catalogue` turns it into the **Status** column, the run
  summary at the top of `TEST-CASES.md` and the **Known failures** table (reasons come from
  `scripts/known-failures.json`).
- `TEST-CASES.md` has one status column per target (**Local**, **Staging**); a run only rewrites its own column.
  `ACC_RESULTS=<file> npm run catalogue` applies a saved results file.
- Status values: `pass`, `fail`, `skip` (precondition not available on that target, the reason is in the report),
  `manual` (not automated — the catalogue says why), `not run` (automated but filtered out of the last run).
