import { Filter, JsSteps, RouteDoc, RouteDraft, Steps, type Json } from '../lib/RouteDraft';

/** Clusters and connections of the staging two-cluster demo (deploy/staging: kafka = A, kafka-b = B). */
export const DemoClusters = {
  a: { connection: 'kafka-a', bootstrap: 'kafka:19092', envTag: 'staging', description: 'Demo cluster A (compose service kafka, same cluster as staging-kafka)' },
  b: { connection: 'kafka-b', bootstrap: 'kafka-b:19092', envTag: 'staging', description: 'Demo cluster B (compose service kafka-b)' },
  bProd: { connection: 'kafka-b-prod', bootstrap: 'kafka-b:19092', envTag: 'prod', description: 'Cluster B tagged prod: routes writing here need a second person (4-eyes)' },
} as const;

export type DemoClusterKey = keyof typeof DemoClusters;

/** Demo topics per connection (3 partitions each; demo-producer creates its input topics with 3 too). */
export const DemoTopics: Record<DemoClusterKey, string[]> = {
  a: ['demo-orders', 'demo-payments-enriched', 'demo-cancelled-a', 'demo-clean'],
  b: ['demo-orders-paid', 'demo-payments', 'demo-paid-b', 'demo-raw', 'demo-b-in', 'demo-b-out'],
  bProd: ['demo-orders-highvalue'],
};

export const DemoPartitions = 3;

/** JS step bodies of the demo routes. */
export const DemoJs = {
  risk: 'function transform(event, ctx) { event.value.riskLevel = event.value.amount >= 200 ? "review" : "ok"; return event; }',
  throwOnBad: 'function transform(event, ctx) { if (event.value.bad) { throw new Error("bad reading " + event.value.id); } event.value.validated = true; return event; }',
} as const;

/** A saved Studio test case (route test-cases API). */
export type DemoCase = { branchId: string; name: string; inputEvent: Json; expectedOutcome: 'passed' | 'filtered' | 'dropped' | 'error'; expectedOutput?: Json };

/** One demo route: draft builder over resolved connection ids, plus its saved test cases. */
export type DemoRoute = { name: string; description: string; build: (conn: Record<DemoClusterKey, string>) => Json; cases: DemoCase[] };

const order = (eventType: string, extra: Json = {}): Json => ({
  seq: 1, orderId: 'ord-000042', eventType, customer: { id: 'cus-101', email: 'anna.kowalski@example.com' }, amount: 129.9, currency: 'EUR', items: 2, ts: '2026-10-18T10:00:00.000Z', ...extra,
});
/** Pinned input events carry headers as a list; expected outputs as a map (the preview trace shape). */
const keyed = (value: Json, key = 'ord-000042'): Json => ({ key, value, headers: [] });
const out = (value: Json, key: string | null = 'ord-000042'): Json => ({ key, value, headers: {} });
const byOrderId = { key: { mode: 'field', field: 'value.orderId' } } as const;

const payment = { seq: 7, paymentId: 'pay-000007', orderId: 'ord-000042', amount_cents: 12990, currency: 'EUR', method: 'card', status: 'captured', internal_debug: { gateway: 'psp-sim', latencyMs: 80 }, ts: '2026-10-18T10:00:01.000Z' };
const enriched = (p: Json, amount: number, risk: string): Json => {
  const { status, internal_debug: _drop, ...rest } = p as Json & { status: string };
  return { ...rest, amount, paymentStatus: status, riskLevel: risk };
};
const reading = (bad: boolean): Json => ({ seq: 3, id: 'rd-000003', sensor: 'line-1/temp', reading: 21.5, bad, ts: '2026-10-18T10:00:02.000Z' });

/** The demo routes (prefix demo-: never swept by the acceptance cleanup). */
export const DemoRoutes: DemoRoute[] = [
  {
    name: 'demo-a-to-b-paid',
    description: 'A demo-orders -> B demo-orders-paid: only OrderPaid, customer email masked, key = orderId',
    build: (c) => new RouteDraft('demo-a-to-b-paid', c.a, ['demo-orders'])
      .branch({ filter: Filter.cond('value.eventType', 'eq', 'OrderPaid'), steps: [{ id: 'mask-email', ...Steps.mask('value.customer.email', 'email') }], topic: 'demo-orders-paid', connectionId: c.b, destination: byOrderId })
      .startFrom(RouteDoc.start.latest).delivery(RouteDoc.delivery.atLeastOnce).build(),
    cases: [
      { branchId: 'main', name: 'OrderPaid passes with masked email', inputEvent: keyed(order('OrderPaid'), 'any-key'), expectedOutcome: 'passed', expectedOutput: out(order('OrderPaid', { customer: { id: 'cus-101', email: 'a************@example.com' } })) },
      { branchId: 'main', name: 'OrderCreated is filtered out', inputEvent: keyed(order('OrderCreated')), expectedOutcome: 'filtered' },
    ],
  },
  {
    name: 'demo-b-to-a-enrich',
    description: 'B demo-payments -> A demo-payments-enriched: rename status, computed amount, drop internal_debug, JS riskLevel',
    build: (c) => new RouteDraft('demo-b-to-a-enrich', c.b, ['demo-payments'])
      .branch({
        steps: [
          { id: 'rename-status', ...Steps.rename('value.status', 'value.paymentStatus') },
          { id: 'amount', kind: 'set', field: 'value.amount', source: { from: 'formula', expr: 'round(value.amount_cents / 100, 2)' } },
          { id: 'drop-debug', ...Steps.remove('value.internal_debug') },
          JsSteps.step(DemoJs.risk, 'risk-js'),
        ],
        topic: 'demo-payments-enriched', connectionId: c.a,
      })
      .startFrom(RouteDoc.start.latest).delivery(RouteDoc.delivery.atLeastOnce).build(),
    cases: [
      { branchId: 'main', name: 'payment is enriched (ok risk)', inputEvent: keyed(payment), expectedOutcome: 'passed', expectedOutput: out(enriched(payment, 129.9, 'ok')) },
      { branchId: 'main', name: 'large payment gets riskLevel review', inputEvent: keyed({ ...payment, amount_cents: 25000 }), expectedOutcome: 'passed', expectedOutput: out(enriched({ ...payment, amount_cents: 25000 }, 250, 'review')) },
    ],
  },
  {
    name: 'demo-a-branches',
    description: 'A demo-orders: OrderPaid -> B demo-paid-b, OrderCancelled -> A demo-cancelled-a, others unmatched',
    build: (c) => new RouteDraft('demo-a-branches', c.a, ['demo-orders'])
      .branch({ id: 'paid', filter: Filter.cond('value.eventType', 'eq', 'OrderPaid'), topic: 'demo-paid-b', connectionId: c.b, destination: byOrderId })
      .branch({ id: 'cancelled', filter: Filter.cond('value.eventType', 'eq', 'OrderCancelled'), topic: 'demo-cancelled-a', connectionId: c.a, destination: byOrderId })
      .startFrom(RouteDoc.start.latest).delivery(RouteDoc.delivery.atLeastOnce).build(),
    cases: [
      { branchId: 'paid', name: 'OrderPaid goes to B demo-paid-b', inputEvent: keyed(order('OrderPaid')), expectedOutcome: 'passed', expectedOutput: out(order('OrderPaid')) },
      { branchId: 'cancelled', name: 'OrderCancelled goes to A demo-cancelled-a', inputEvent: keyed(order('OrderCancelled')), expectedOutcome: 'passed', expectedOutput: out(order('OrderCancelled')) },
      { branchId: 'paid', name: 'OrderShipped matches no branch (paid)', inputEvent: keyed(order('OrderShipped')), expectedOutcome: 'filtered' },
    ],
  },
  {
    name: 'demo-b-dlq',
    description: 'B demo-raw -> A demo-clean: JS throws on bad:true -> dead letter (router.dlq.demo-b-dlq on B)',
    build: (c) => new RouteDraft('demo-b-dlq', c.b, ['demo-raw'])
      .branch({ steps: [JsSteps.step(DemoJs.throwOnBad, 'validate-js')], topic: 'demo-clean', connectionId: c.a })
      .startFrom(RouteDoc.start.latest).delivery(RouteDoc.delivery.atLeastOnce).onError(RouteDoc.onError.dlq).build(),
    cases: [
      { branchId: 'main', name: 'good reading passes validated', inputEvent: keyed(reading(false), 'rd-000003'), expectedOutcome: 'passed', expectedOutput: out({ ...reading(false), validated: true }, 'rd-000003') },
      { branchId: 'main', name: 'bad reading throws (dead letter)', inputEvent: keyed(reading(true), 'rd-000003'), expectedOutcome: 'error' },
    ],
  },
  {
    name: 'demo-b-local',
    description: 'B demo-b-in -> B demo-b-out on one connection: exactly_once (transactions)',
    build: (c) => new RouteDraft('demo-b-local', c.b, ['demo-b-in'])
      .branch({ steps: [{ id: 'tag', ...Steps.setConstant('value.via', 'demo-b-local') }], topic: 'demo-b-out', connectionId: c.b })
      .startFrom(RouteDoc.start.latest).delivery(RouteDoc.delivery.exactlyOnce).build(),
    cases: [
      { branchId: 'main', name: 'ping is tagged', inputEvent: keyed({ seq: 1, id: 'loc-000001', kind: 'heartbeat' }, 'loc-000001'), expectedOutcome: 'passed', expectedOutput: out({ seq: 1, id: 'loc-000001', kind: 'heartbeat', via: 'demo-b-local' }, 'loc-000001') },
    ],
  },
];

/** 4-eyes demo: a route into the prod-tagged connection; v1 approved by the approver, v2 left pending. */
export const DemoGate = {
  name: 'demo-approval-gate',
  description: 'A demo-orders (amount >= threshold) -> B (prod-tagged) demo-orders-highvalue; deploys need a second person',
  approvedThreshold: 100,
  pendingThreshold: 200,
  build: (c: Record<DemoClusterKey, string>, threshold: number): Json => new RouteDraft('demo-approval-gate', c.a, ['demo-orders'])
    .branch({ filter: Filter.cond('value.amount', 'gte', threshold), steps: [{ id: 'mask-email', ...Steps.mask('value.customer.email', 'email') }], topic: 'demo-orders-highvalue', connectionId: c.bProd, destination: byOrderId })
    .startFrom(RouteDoc.start.latest).delivery(RouteDoc.delivery.atLeastOnce).build(),
} as const;

/** Alert rule on a demo route (error rate of the DLQ demo is ~10%, so it fires). */
export const DemoAlert = { route: 'demo-b-dlq', name: 'demo-b-dlq error rate above 5%', kind: 'error_rate_above', threshold: 5, windowMinutes: 5 } as const;

/** Second Dex identity for 4-eyes approvals (keys in the staging credentials file). */
export const ApproverKeys = { email: 'STAGING_APPROVER_EMAIL', password: 'STAGING_APPROVER_PASSWORD', role: 'approver' } as const;
