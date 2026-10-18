# Web E2E (Playwright)

Runs against a live stack: `deploy/dev/dev.sh up` (vite on :5173 with `VITE_DATA_SOURCE=http`, api on :8080, dev login).

```sh
cd web/e2e && npm install
npx playwright install chromium        # once; no sudo needed for the browser itself
npx playwright test                    # E2E_BASE_URL overrides http://localhost:5173
```

Missing system libraries (no sudo): `apt-get download` the packages that `ldd` reports missing for the
headless shell, `dpkg-deb -x` them into `~/.local/lib/playwright-deps`, and run with
`LD_LIBRARY_PATH=~/.local/lib/playwright-deps/lib npx playwright test`.

Users created by the tests are named `dev-access-control-e2e-<timestamp>@local.dev`.

`tests/dead-letters.spec.ts` needs the route `dev-dead-letters-orders` (override with `E2E_DLQ_ROUTE`) deployed with a JS
step that throws on `value.bad`, `errorPolicy.onError=dlq`, and ≥ 2 dead letters (produce `{"id":"ord_1","bad":true}`
events to its source topic `dev-dead-letters-in`); otherwise the test is skipped. Each run discards one event.

`tests/dead-letters-p2.spec.ts` (DL-127) uses the same route; the background-job case needs more than 500 unresolved dead
letters (produce ≥ 500 failing events to `dev-dead-letters-in`), otherwise it is skipped. It retries them in a background job
(they fail again and stay) and discards + restores one event.


`tests/monitoring-p2.spec.ts` needs the deployed route `dev-monitoring-alerts` (override with `E2E_MONITORING_ROUTE`); it
resumes the route while a throughput-zero rule fires (up to ~60 s), then pauses it again and deletes its `dev-monitoring-e2e-*` rules.

`tests/connections-p3.spec.ts` (CN-203/204) needs a `kind=demo` connection: it calls `POST /api/v1/onboarding/demo` (monitoring#P3-BE-07)
first; without that endpoint seed one through connections `Service.CreateDemo`, otherwise the test is skipped.

`tests/dead-letters-p3.spec.ts` (DL-209, DL-204, DL-206) needs a `kind=demo` connection (`POST /api/v1/onboarding/demo` when
available, else `demo-orders` from connections `Service.CreateDemo`). Each test creates a `dev-dead-letters-demo-*` route on
it (the API seeds 12 dead letters into its DLQ topic on the in-process demo cluster), fixes the JS step through the API
(draft + deploy) and archives the route at the end; an `exactly_once` twin checks the at-least-once warning and the
"Dead-letter actions" audit preset.

`tests/monitoring-p3.spec.ts` (P3-T-06) needs the deployed route `dev-monitoring-tail` (override with `E2E_MONITORING_TAIL_ROUTE`) reading the
demo connection's `order-events` (the worker's in-process demo generator feeds it); it resumes the route for the live-tail case and pauses it
again, checks the demo data endpoints (`201`/`409`), and starts one replay of the route through the confirm dialog.

`tests/transform-studio-ts14.spec.ts` (TS-14) adds each structure / Kafka-header recipe in the Studio on an uploaded sample
and checks the preview-service output and card examples, then builds split array + header ops + wrap on a `dev-ts14-*`
route (own `dev` connection on `E2E_BOOTSTRAP`, default `127.0.0.1:29092`), deploys it from the builder and verifies the
output events on Kafka with kafkajs (+ `kafkajs-snappy`: workers compress with snappy). Route, connection, topics and
consumer groups are removed at the end.
