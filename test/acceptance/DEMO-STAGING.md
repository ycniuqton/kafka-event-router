# Staging demo: routes between two Kafka clusters

Staging (`http://104.234.195.126`) runs two independent Kafka clusters. All demo resources are named `demo-`
(the acceptance sweeper only removes `acc-`), so they stay until you delete them.

| Cluster | Compose service | In-stack bootstrap | On the host / ssh tunnel |
|---|---|---|---|
| A | `kafka` | `kafka:19092` | `127.0.0.1:29092` |
| B | `kafka-b` | `kafka-b:19092` | `127.0.0.1:29093` |

## Connections (4 in the UI)

| Connection | Points to | Env | Why it exists |
|---|---|---|---|
| `staging-kafka` | cluster A | staging | Older connection used by the `orders-smoke` route. Left as it is. |
| `kafka-a` | cluster A (same broker as `staging-kafka`) | staging | Clearly named source/destination "A" for the demos. |
| `kafka-b` | cluster B | staging | Cluster "B" for the demos. |
| `kafka-b-prod` | cluster B (same broker as `kafka-b`) | **prod** | Only used by `demo-approval-gate`. A route that writes to a `prod` connection needs a second person to approve it (4-eyes). |

`staging-kafka` and `kafka-a` are two connections to the same broker. Routes are tied to a connection id, so
`orders-smoke` keeps `staging-kafka`.

## Demo routes

`demo-producer` sends about 1 event/s to each of `demo-orders` (A), `demo-payments`, `demo-raw` (B, about 10% `bad:true`)
and `demo-b-in` (B). Every route runs with saved Studio test cases: open the route, click **Edit**, and the
**Test cases** panel is under Transform steps. Click **Run** there.

| Route | Flow | What it does | Look at |
|---|---|---|---|
| `demo-a-to-b-paid` | A `demo-orders` → B `demo-orders-paid` | Keeps only `eventType == OrderPaid`, masks `customer.email`, sets the key to `orderId` | Match % (~30%), **Live tail** (input vs output) |
| `demo-b-to-a-enrich` | B `demo-payments` → A `demo-payments-enriched` | Renames `status`→`paymentStatus`, sets `amount = round(amount_cents/100, 2)`, drops `internal_debug`, then a JS step adds `riskLevel` | Studio step-by-step diff |
| `demo-a-branches` | A `demo-orders` → branch `paid`: B `demo-paid-b`; branch `cancelled`: A `demo-cancelled-a` | Other event types match no branch and are dropped | Match % (~45%): events matching no branch are counted as filtered |
| `demo-b-dlq` | B `demo-raw` → A `demo-clean` | JS throws on `bad:true`, and those events go to the dead-letter topic `router.dlq.demo-b-dlq` (on B) | **Dead letters** (one entry was retried and failed again), the alert `demo-b-dlq error rate above 5%` (firing) |
| `demo-b-local` | B `demo-b-in` → B `demo-b-out` | Sets `via`. Uses **exactly_once** (Kafka transactions) | Compare with the cross-cluster routes |
| `demo-approval-gate` | A `demo-orders` → `kafka-b-prod` `demo-orders-highvalue` | Keeps orders with `amount >= 100` (v3, approved by `staging-approver`) | **Approvals** (header): v4 (`>= 200`) is waiting for approval |

**Delivery guarantees.** `exactly_once` works only when the source, all destinations and the DLQ use the same
connection (`demo-b-local`). Cross-cluster routes run **at_least_once**: after a crash or rebalance an event can be
written twice, but it is never lost. The validator rejects `exactly_once` on cross-cluster routes
(`exactly_once_cross_cluster`).

**4-eyes.** Nobody can approve their own request, admins included. To approve or reject the pending v4, sign in
as `STAGING_APPROVER_EMAIL` / `STAGING_APPROVER_PASSWORD` from `~/.secrets/event-router-staging.env` (role
`approver`). Sign in as the admin (`STAGING_LOGIN_*`) to request changes.

## Send your own event

```sh
ssh root@104.234.195.126
K="docker run --rm -i --network event-router-staging-backend apache/kafka:3.9.1 /opt/kafka/bin"
# an order on A (demo-a-to-b-paid, demo-a-branches, demo-approval-gate)
echo 'ord-me-1|{"orderId":"ord-me-1","eventType":"OrderPaid","customer":{"email":"me@example.com"},"amount":250}' |
  $K/kafka-console-producer.sh --bootstrap-server kafka:19092 --topic demo-orders --property parse.key=true --property key.separator='|'
# a bad reading on B (demo-b-dlq -> Dead letters)
echo '{"id":"rd-me-1","reading":1,"bad":true}' | $K/kafka-console-producer.sh --bootstrap-server kafka-b:19092 --topic demo-raw
# read a destination (Ctrl-C to stop)
$K/kafka-console-consumer.sh --bootstrap-server kafka-b:19092 --topic demo-orders-paid --property print.key=true
```

From your machine, use `deploy/dev/tunnel.sh start` and connect to `127.0.0.1:29092` (A) / `127.0.0.1:29093` (B).

## Stop / start the demo traffic

```sh
cd /opt/event-router/src
C="docker compose -f deploy/staging/docker-compose.staging.yml --env-file /opt/event-router/.env"
$C stop demo-producer     # stop (deploy.sh or `up -d` starts it again)
$C start demo-producer
# keep it off: add DEMO_PRODUCER_REPLICAS=0 to /opt/event-router/.env and redeploy; rate: DEMO_PRODUCER_RATE=<ev/s>
```

## Re-run / re-check (from `test/acceptance`, `ACC_TARGET=staging`)

```sh
npx playwright test -c demo/demo.config.ts seed   # idempotent find-or-create of everything above
npx playwright test -c demo/demo.config.ts ui     # browser walk-through, screenshots in report/demo-screens/
ssh root@104.234.195.126 'bash -s' < demo/verify-kafka.sh   # end-to-end Kafka check of every route
```
