import { expect, test } from '../../lib/Fixtures';
import { Paths } from '../../lib/Platform';
import { Filter, Steps } from '../../lib/RouteDraft';
import { StudioRunner } from '../../lib/Studio';

/** Live preview and testing (UI spec §9, SysDesign §10). */
const Preview = { sessions: (id: string) => `/api/v1/preview/sessions/${id}`, maxEvents: 1000 } as const;

/** Order events seeded on the source topic. */
class Orders {
  static readonly types = ['OrderPaid', 'OrderCreated', 'OrderPaid', 'OrderRefunded', 'OrderPaid', 'OrderCreated'];

  static events(): { key: string; value: any }[] {
    return Orders.types.map((eventType, i) => ({ key: `o-${i}`, value: { id: i, eventType, amount: (i + 1) * 50, country: i % 2 ? 'VN' : 'DE', email: `user${i}@mail.com` } }));
  }
}

test.describe('live preview', () => {
  test('TC-PRV-01 latest N from the topic loads samples without a consumer group or committed offsets', async ({ ws, api }) => {
    const conn = await ws.pipe();
    await ws.kafka.produce(ws.names.inTopic, Orders.events());
    const res = await api.post(Paths.preview.samples, { connectionId: conn, topics: [ws.names.inTopic], mode: 'latest', count: 50 });
    expect(res.status, res.describe()).toBeLessThan(300);
    expect(res.body.session.loaded).toBe(Orders.types.length);
    expect(res.body.events.map((e: { key: string }) => e.key).sort()).toEqual(Orders.events().map((e) => e.key).sort());
    const groups = await ws.kafka.groupsReading(ws.names.inTopic);
    expect(groups, 'no consumer group may own offsets on the sampled topic').toEqual([]);
  });

  test('TC-PRV-02 uploaded events create a session for empty dev topics', async ({ ws }) => {
    const id = await ws.uploadSession([{ value: { a: 1 } }, { value: { a: 2 } }]);
    expect(id).toMatch(/^ps_/);
  });

  test('TC-PRV-03 more than 1,000 samples are refused or clamped to the hard limit', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const res = await api.post(Paths.preview.samples, { connectionId: conn, topics: [ws.names.inTopic], mode: 'latest', count: 5000 });
    if (res.ok) expect(res.body.session.loaded).toBeLessThanOrEqual(Preview.maxEvents);
    else expect(res.status).toBe(422);
  });

  test('TC-PRV-04 a run returns per-event status and the summary counts', async ({ ws }) => {
    const filter = Filter.cond('value.eventType', 'eq', 'OrderPaid');
    const result = await new StudioRunner(ws).run([{ kind: 'js', code: 'function transform(e){ if (e.value.amount === 250) throw new Error("boom"); return e; }' }], Orders.events(), filter);
    expect(result.summary).toMatchObject({ loaded: 6, passed: 2, filtered: 3, errors: 1 });
    expect(result.statuses()).toEqual(['passed', 'filtered', 'passed', 'filtered', 'error', 'filtered']);
  });

  test('TC-PRV-05 why filtered: the trace records the result of every filter node', async ({ ws }) => {
    const filter = Filter.and(Filter.cond('value.eventType', 'eq', 'OrderPaid'), Filter.cond('value.country', 'eq', 'VN'));
    const result = await new StudioRunner(ws).run([], [Orders.events()[0]], filter);
    const nodes = result.results[0].branches[0].filter;
    expect(nodes.find((n: { path: string }) => n.path.endsWith('/children/0'))?.result).toBe(true);
    expect(nodes.find((n: { path: string }) => n.path.endsWith('/children/1'))?.result).toBe(false);
    expect(nodes.find((n: { path: string }) => n.path.endsWith('/filter'))?.result).toBe(false);
  });

  test('TC-PRV-06 field schema inference: paths, types and top values with frequency', async ({ ws, api }) => {
    const id = await ws.uploadSession(Orders.events());
    const res = await api.get(`${Preview.sessions(id)}/schema`);
    expect(res.status, res.describe()).toBe(200);
    const text = JSON.stringify(res.body);
    expect(text).toContain('eventType');
    expect(text).toContain('OrderPaid');
    expect(res.body.samples).toBe(6);
  });

  test('TC-PRV-08 search the loaded samples by text', async ({ ws, api }) => {
    const id = await ws.uploadSession(Orders.events());
    const res = await api.get(`${Preview.sessions(id)}/events?q=OrderRefunded`);
    expect(res.status, res.describe()).toBe(200);
    expect(res.body.events.map((e: { value: { eventType: string } }) => e.value.eventType)).toEqual(['OrderRefunded']);
  });

  test('TC-PRV-09 raw view: exact bytes, key and headers of each sample', async ({ ws, api }) => {
    const conn = await ws.pipe();
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'k-raw', value: '{"a": 1,  "b":"x"}', headers: { traceId: 't-9' } }]);
    const res = await api.post(Paths.preview.samples, { connectionId: conn, topics: [ws.names.inTopic], mode: 'latest', count: 1 });
    const sample = res.body.events[0];
    expect(sample.raw).toMatchObject({ key: 'k-raw', value: '{"a": 1,  "b":"x"}' });
    expect(sample.headers).toEqual([expect.objectContaining({ key: 'traceId' })]);
    expect(sample.value).toEqual({ a: 1, b: 'x' });
  });

  test('TC-PRV-10 load samples from a time range', async ({ ws, api }) => {
    const conn = await ws.pipe();
    await ws.kafka.produce(ws.names.inTopic, Orders.events());
    const from = new Date(Date.now() - 10 * 60_000).toISOString();
    const to = new Date(Date.now() + 60_000).toISOString();
    const res = await api.post(Paths.preview.samples, { connectionId: conn, topics: [ws.names.inTopic], mode: 'time_range', from, to, count: 100 });
    expect(res.status, res.describe()).toBeLessThan(300);
    expect(res.body.session.loaded).toBe(6);
  });

  test('TC-PRV-07 test run writes outputs to a temporary test topic, never the real destination', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const draft = ws.draft(conn, [{ topic: ws.names.outTopic, steps: [Steps.setConstant('value.tested', true)] }]).build();
    const routeId = await ws.createRoute(draft);
    const session = await ws.uploadSession(Orders.events().slice(0, 2));
    const res = await api.post(Paths.preview.testRuns, { sessionId: session, config: draft, writeToTopic: true, routeId });
    expect(res.status, res.describe()).toBe(200);
    const topic = res.body.testTopic?.topic as string;
    expect(topic).toMatch(new RegExp(`^router\\.test\\.${ws.names.route}\\.`));
    ws.resources.topic(topic);
    expect(res.body.testTopic.produced).toBe(2);
    expect((await ws.kafka.consume(topic, 2, 20_000)).map((r) => r.value.tested)).toEqual([true, true]);
    expect(await ws.kafka.drain(ws.names.outTopic, 3_000)).toHaveLength(0);
  });
});
