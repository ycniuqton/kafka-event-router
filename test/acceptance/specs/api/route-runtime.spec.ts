import type { AccRecord } from '../../lib/Kafka';
import { expect, test } from '../../lib/Fixtures';
import { Paths, type Workspace } from '../../lib/Platform';
import { Filter, JsSteps, RouteDoc, Steps } from '../../lib/RouteDraft';
import { StudioRunner } from '../../lib/Studio';

/** Route runtime: delivery, ordering, error policies, DLQ, hot reload, branches, exactly-once, replay (SysDesign §8–§9). */
const Run = { dlqHeaders: ['router.route_id', 'router.version', 'router.step', 'router.error_type', 'router.error_message', 'router.source_topic', 'router.source_partition', 'router.source_offset', 'router.failed_at'] } as const;

/** Event fixtures for the runtime scenarios. */
class Events {
  static orders(n: number, from = 0): { key: string; value: any }[] {
    return Array.from({ length: n }, (_, i) => ({ key: `o-${from + i}`, value: { id: from + i, eventType: (from + i) % 2 ? 'OrderCreated' : 'OrderPaid', amount: 10 * (from + i) } }));
  }

  static keys(records: AccRecord[]): string[] {
    return [...new Set(records.map((r) => r.key ?? ''))].sort();
  }
}

/** Reads the route's dead letters through the API until `count` are listed. */
async function deadLetters(ws: Workspace, routeId: string, count: number): Promise<any[]> {
  let items: any[] = [];
  await expect.poll(async () => (items = (await ws.api.get(`${Paths.dlq(routeId)}?limit=50`)).body?.items ?? []).length, { timeout: 60_000, intervals: [2_000] }).toBeGreaterThanOrEqual(count);
  return items;
}

test.describe('route runtime', () => {
  test('TC-RUN-01 filter & forward delivers only matching events with their keys', async ({ ws }) => {
    const conn = await ws.pipe();
    await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, filter: Filter.cond('value.eventType', 'eq', 'OrderPaid') }]));
    await ws.kafka.produce(ws.names.inTopic, Events.orders(6));
    const out = await ws.kafka.consume(ws.names.outTopic, 3);
    expect(Events.keys(out)).toEqual(['o-0', 'o-2', 'o-4']);
    expect(out.every((r) => r.value.eventType === 'OrderPaid')).toBe(true);
    expect(await ws.kafka.drain(ws.names.outTopic)).toHaveLength(3);
  });

  test('TC-RUN-02 parity: the worker produces exactly what the preview showed', async ({ ws }) => {
    const conn = await ws.pipe();
    const steps = [Steps.rename('value.amount', 'value.total'), Steps.mask('value.eventType', 'last4'), { kind: 'set', field: 'value.big', source: { from: 'formula', expr: 'round(value.total * 1.5, 1)' } }];
    const events = Events.orders(4);
    const preview = (await new StudioRunner(ws).run(steps, events)).values;
    await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps }]));
    await ws.kafka.produce(ws.names.inTopic, events);
    const out = await ws.kafka.consume(ws.names.outTopic, 4);
    expect(out.map((r) => r.value)).toEqual(preview);
  });

  test('TC-RUN-03 at-least-once across pause / resume: no event is lost', async ({ ws }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn));
    await ws.kafka.produce(ws.names.inTopic, Events.orders(20));
    await ws.kafka.consume(ws.names.outTopic, 20);
    expect((await ws.pause(id)).status).toBe(200);
    await ws.waitObserved(id, 'paused');
    await ws.kafka.produce(ws.names.inTopic, Events.orders(20, 20));
    expect((await ws.resume(id)).status).toBe(200);
    const out = await ws.kafka.consume(ws.names.outTopic, 40);
    expect(Events.keys(out)).toEqual(Events.orders(40).map((e) => e.key).sort());
  });

  test('TC-RUN-04 ordering is preserved per source partition and per key', async ({ ws }) => {
    const conn = await ws.pipe(3);
    await ws.runningRoute(ws.draft(conn));
    const messages = Array.from({ length: 30 }, (_, i) => ({ key: `k-${i % 3}`, value: { seq: Math.floor(i / 3), k: i % 3 }, partition: i % 3 }));
    await ws.kafka.produce(ws.names.inTopic, messages);
    const out = await ws.kafka.consume(ws.names.outTopic, 30);
    for (const key of ['k-0', 'k-1', 'k-2']) {
      const seqs = out.filter((r) => r.key === key).map((r) => r.value.seq);
      expect(seqs, key).toEqual([...seqs].sort((a, b) => a - b));
      expect(new Set(out.filter((r) => r.key === key).map((r) => r.partition)).size, `${key} stays on one partition`).toBe(1);
    }
  });

  test('TC-RUN-05 a JS step error sends the original record to the DLQ with router.* headers', async ({ ws }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.step(JsSteps.throwOnBad)] }]));
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'bad-1', value: { id: 1, bad: true }, headers: { traceId: 't-1' } }, { key: 'ok-2', value: { id: 2 } }]);
    expect(Events.keys(await ws.kafka.consume(ws.names.outTopic, 1))).toEqual(['ok-2']);
    const [dead] = await ws.kafka.consume(ws.names.dlqTopic(), 1);
    expect(dead.key).toBe('bad-1');
    expect(dead.value).toEqual({ id: 1, bad: true });
    expect(dead.headers.traceId).toBe('t-1');
    for (const h of Run.dlqHeaders) expect(Object.keys(dead.headers), h).toContain(h);
    expect(dead.headers['router.route_id']).toBe(id);
    expect(dead.headers['router.error_message']).toContain('bad event 1');
  });

  test('TC-RUN-06 an undecodable record goes to the DLQ as decode error and the route continues', async ({ ws }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn));
    await ws.kafka.produceRaw(ws.names.inTopic, ['{not json', '{"id":2}']);
    expect((await ws.kafka.consume(ws.names.outTopic, 1)).map((r) => r.value)).toEqual([{ id: 2 }]);
    const [item] = await deadLetters(ws, id, 1);
    expect(item.errorType).toMatch(/decode/);
  });

  test('TC-RUN-07 onError=skip drops the failing event, counts it and continues without a DLQ record', async ({ ws }) => {
    const conn = await ws.pipe();
    await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.step(JsSteps.throwOnBad)] }]).onError(RouteDoc.onError.skip));
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'bad', value: { id: 1, bad: true } }, { key: 'ok', value: { id: 2 } }]);
    expect(Events.keys(await ws.kafka.consume(ws.names.outTopic, 1))).toEqual(['ok']);
    expect(await ws.kafka.topicExists(ws.names.dlqTopic()) ? await ws.kafka.drain(ws.names.dlqTopic(), 3_000) : []).toHaveLength(0);
  });

  test('TC-RUN-08 onError=pause halts the route on the first failing event; TC-MON-02 it needs attention', async ({ ws }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.step(JsSteps.throwOnBad)] }]).onError(RouteDoc.onError.pause));
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'ok-1', value: { id: 1 } }, { key: 'bad', value: { id: 2, bad: true } }, { key: 'ok-3', value: { id: 3 } }]);
    await expect.poll(async () => (await ws.route(id)).observedState, { timeout: 60_000 }).toMatch(/error|paused/);
    expect(Events.keys(await ws.kafka.drain(ws.names.outTopic))).toEqual(['ok-1']);
    const attention = await ws.api.get('/api/v1/dashboard/attention');
    expect(JSON.stringify(attention.body), 'TC-MON-02 a halted route needs attention').toContain(id);
  });

  test('TC-RUN-09 a JS timeout is retried, then dead-lettered as a timeout', async ({ ws }) => {
    const conn = await ws.pipe();
    const loop = 'function transform(event, ctx) { if (event.value.slow) { while (true) {} } return event; }';
    const id = await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.step(loop)] }]).onError(RouteDoc.onError.dlq, { maxRetries: 1 }));
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'slow', value: { slow: true } }, { key: 'fast', value: { id: 2 } }]);
    expect(Events.keys(await ws.kafka.consume(ws.names.outTopic, 1))).toEqual(['fast']);
    const [item] = await deadLetters(ws, id, 1);
    expect(item.errorType).toMatch(/timeout/);
  });

  test('TC-RUN-10 hot reload: a new version applies to later events without losing any', async ({ ws }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.tagVersion(1)] }]));
    await ws.kafka.produce(ws.names.inTopic, Events.orders(5));
    await ws.kafka.consume(ws.names.outTopic, 5);
    expect((await ws.saveDraft(id, ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.tagVersion(2)] }]))).status).toBe(200);
    expect((await ws.deploy(id, 'v2')).status).toBe(202);
    await ws.waitRunningVersion(id, 2);
    await ws.kafka.produce(ws.names.inTopic, Events.orders(5, 5));
    const out = await ws.kafka.consume(ws.names.outTopic, 10);
    expect(Events.keys(out)).toHaveLength(10);
    expect(out.filter((r) => Number(r.key?.slice(2)) >= 5).every((r) => r.value.v === 2)).toBe(true);
  });

  test('TC-RUN-11 branches: each branch filters and delivers to its own destination', async ({ ws }) => {
    const conn = await ws.pipe();
    const refunds = ws.names.of('refunds');
    await ws.topics(conn, [refunds]);
    await ws.runningRoute(
      ws.draft(conn, [
        { topic: ws.names.outTopic, filter: Filter.cond('value.eventType', 'eq', 'OrderPaid') },
        { topic: refunds, filter: Filter.cond('value.eventType', 'eq', 'OrderCreated'), steps: [Steps.setConstant('value.branch', 2)] },
      ]),
    );
    await ws.kafka.produce(ws.names.inTopic, Events.orders(4));
    expect(Events.keys(await ws.kafka.consume(ws.names.outTopic, 2))).toEqual(['o-0', 'o-2']);
    const second = await ws.kafka.consume(refunds, 2);
    expect(Events.keys(second)).toEqual(['o-1', 'o-3']);
    expect(second.every((r) => r.value.branch === 2)).toBe(true);
  });

  test('TC-RUN-12 exactly-once (same cluster): a read_committed consumer sees every event once', async ({ ws }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn).delivery(RouteDoc.delivery.exactlyOnce));
    await ws.kafka.produce(ws.names.inTopic, Events.orders(10));
    await ws.kafka.consume(ws.names.outTopic, 10);
    await ws.pause(id);
    await ws.waitObserved(id, 'paused');
    await ws.resume(id);
    await ws.kafka.produce(ws.names.inTopic, Events.orders(5, 10));
    const out = await ws.kafka.consume(ws.names.outTopic, 15);
    const all = await ws.kafka.drain(ws.names.outTopic);
    expect(Events.keys(out)).toHaveLength(15);
    expect(all).toHaveLength(15);
  });

  test('TC-RUN-13 replay re-reads from a point in time; the destination receives duplicates', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn));
    const from = new Date(Date.now() - 1_000).toISOString();
    await ws.kafka.produce(ws.names.inTopic, Events.orders(3));
    await ws.kafka.consume(ws.names.outTopic, 3);
    const refused = await api.post(Paths.routeAction(id, 'replay'), { from, confirm: false });
    expect(refused.status, 'replay needs confirmation').toBeGreaterThanOrEqual(400);
    const replay = await api.post(Paths.routeAction(id, 'replay'), { from, confirm: true });
    expect(replay.status, replay.describe()).toBe(202);
    await expect.poll(async () => (await api.get(`/api/v1/routes/${id}/replays/${replay.body.replayId}`)).body.status, { timeout: 90_000, intervals: [2_000] }).toBe('done');
    expect(await ws.kafka.consume(ws.names.outTopic, 6, 60_000)).toHaveLength(6);
  });

  test('TC-RUN-14 destination key from a field and by-key partitioning', async ({ ws }) => {
    const conn = await ws.pipe();
    await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, destination: { key: { mode: 'field', field: 'value.id' } } }]));
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'orig', value: { id: 'cust-9' } }]);
    expect((await ws.kafka.consume(ws.names.outTopic, 1))[0].key).toBe('cust-9');
  });

  test('TC-RUN-15 destination headers: keep selected and add new', async ({ ws }) => {
    const conn = await ws.pipe();
    await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, destination: { headers: { mode: 'keep_selected', keep: ['traceId'], add: [{ key: 'x-router', value: 'acc' }] } } }]));
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'h', value: { id: 1 }, headers: { traceId: 't-1', secret: 's' } }]);
    expect((await ws.kafka.consume(ws.names.outTopic, 1))[0].headers).toEqual({ traceId: 't-1', 'x-router': 'acc' });
  });

  test('TC-RUN-16 start from latest skips events that were already on the topic', async ({ ws }) => {
    const conn = await ws.pipe();
    await ws.kafka.produce(ws.names.inTopic, Events.orders(3));
    await ws.runningRoute(ws.draft(conn).startFrom(RouteDoc.start.latest));
    await new Promise((r) => setTimeout(r, 5_000));
    await ws.kafka.produce(ws.names.inTopic, Events.orders(2, 100));
    const out = await ws.kafka.consume(ws.names.outTopic, 2);
    expect(Events.keys(out)).toEqual(['o-100', 'o-101']);
    expect(await ws.kafka.drain(ws.names.outTopic)).toHaveLength(2);
  });

  test('TC-RUN-20 isolation: a route stuck in timeouts does not stall another route', async ({ ws }) => {
    const conn = await ws.pipe();
    const other = ws.names.of('other');
    await ws.topics(conn, [other]);
    ws.resources.group(ws.names.group(other), `router.${other}`);
    await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.step(JsSteps.infiniteLoop)] }]));
    await ws.runningRoute(ws.draft(conn, [{ topic: other }], other));
    const started = Date.now();
    await ws.kafka.produce(ws.names.inTopic, Events.orders(20));
    expect(await ws.kafka.consume(other, 20, 30_000)).toHaveLength(20);
    expect(Date.now() - started).toBeLessThan(30_000);
  });
});
