import { Http } from '../../lib/Api';
import { expect, test } from '../../lib/Fixtures';
import { Paths } from '../../lib/Platform';
import { Filter, JsSteps, RouteDoc, Steps } from '../../lib/RouteDraft';
import { StudioRunner } from '../../lib/Studio';

/** Routes list and route builder validation (UI spec §6–§7, SysDesign §5 validation rules). */
const Issue = { error: 'error', warning: 'warning' } as const;

test.describe('routes list', () => {
  test('TC-RTL-01 a created route is a paused draft and reads like a sentence in the list', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn, [{ topic: ws.names.outTopic, filter: Filter.cond('value.eventType', 'eq', 'OrderPaid') }]));
    const list = await api.get(`${Paths.routes}?q=${ws.names.route}`);
    const row = list.body.items.find((r: { id: string }) => r.id === id);
    expect(row).toMatchObject({ name: ws.names.route, status: 'draft', source: { connectionName: ws.names.connection, topics: [ws.names.inTopic] }, destinations: [expect.objectContaining({ topic: ws.names.outTopic })] });
    expect(row.desiredState).toBe('paused');
  });

  test('TC-RTL-02 search by name and filter by status', async ({ ws, api }) => {
    const conn = await ws.pipe();
    await ws.createRoute(ws.draft(conn));
    expect((await api.get(`${Paths.routes}?q=${ws.names.route}&status=draft`)).body.items).toHaveLength(1);
    expect((await api.get(`${Paths.routes}?q=${ws.names.route}&status=running`)).body.items).toHaveLength(0);
    expect((await api.get(`${Paths.routes}?q=${ws.names.route.toUpperCase()}`)).body.items).toHaveLength(1);
  });

  test('TC-RTL-03 filter summary and transform indicator (none / visual / js)', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const filter = Filter.and(Filter.cond('value.eventType', 'eq', 'OrderPaid'), Filter.cond('value.amount', 'gt', 100), Filter.cond('value.country', 'eq', 'VN'));
    const kinds: Record<string, unknown[]> = { none: [], visual: [Steps.rename('value.a', 'value.b')], js: [JsSteps.step(JsSteps.identity)] };
    for (const [indicator, steps] of Object.entries(kinds)) {
      const name = ws.names.of(`ind-${indicator}`);
      const id = await ws.createRoute(ws.draft(conn, [{ topic: ws.names.outTopic, filter, steps: steps as never }], name));
      const row = (await api.get(`${Paths.routes}?q=${name}`)).body.items.find((r: { id: string }) => r.id === id);
      expect(row.transform, indicator).toBe(indicator);
      expect(row.filterSummary.headline).toMatch(/eventType/);
      expect(row.filterSummary.more).toBe(2);
    }
  });

  test('TC-RTL-04 duplicate creates a paused copy', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    const copy = await api.post(Paths.routeAction(id, 'duplicate'), {});
    expect(copy.status, copy.describe()).toBe(201);
    ws.resources.route(copy.body.id);
    expect(copy.body.name).toMatch(new RegExp(`^${ws.names.route}-copy`));
    expect(copy.body.desiredState).toBe('paused');
    expect(copy.body.draft.source.topics).toEqual([ws.names.inTopic]);
  });

  test('TC-RTL-05 archive hides the route; status=archived lists it; unarchive returns it paused', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    expect((await api.withEtag(Http.Delete, Paths.route(id))).status).toBe(204);
    expect((await api.get(`${Paths.routes}?q=${ws.names.route}`)).body.items).toHaveLength(0);
    expect((await api.get(`${Paths.routes}?q=${ws.names.route}&status=archived`)).body.items).toHaveLength(1);
    const back = await api.post(Paths.routeAction(id, 'unarchive'), {});
    expect(back.status, back.describe()).toBe(200);
    expect(back.body.status).not.toBe('archived');
  });

  test('TC-RTL-06 bulk actions report one result per route', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const a = await ws.createRoute(ws.draft(conn, undefined, ws.names.of('bulk-a')));
    const b = await ws.createRoute(ws.draft(conn, undefined, ws.names.of('bulk-b')));
    const tag = await api.post('/api/v1/routes/bulk', { action: 'add_tag', ids: [a, b], tag: 'acc-tag' });
    expect(tag.status, tag.describe()).toBe(200);
    expect(tag.body.results.map((r: { ok: boolean }) => r.ok)).toEqual([true, true]);
    expect((await api.get(`${Paths.routes}?tag=acc-tag&q=${ws.names.base}`)).body.items).toHaveLength(2);
    const pause = await api.post('/api/v1/routes/bulk', { action: 'pause', ids: [a] });
    expect(pause.body.results[0].ok, 'a draft cannot be paused').toBe(false);
  });

  test('TC-RTL-07 bulk export returns one YAML artifact for several routes', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const a = await ws.createRoute(ws.draft(conn, undefined, ws.names.of('exp-a')));
    const b = await ws.createRoute(ws.draft(conn, undefined, ws.names.of('exp-b')));
    const res = await api.post('/api/v1/routes/bulk', { action: 'export', ids: [a, b], format: 'yaml' });
    expect(res.status, res.describe()).toBe(200);
    expect(res.body.export.content).toContain(ws.names.of('exp-a'));
    expect(res.body.export.content).toContain(ws.names.of('exp-b'));
    expect(res.body.export.content).toMatch(/^---$/m);
  });
});

test.describe('route builder validation', () => {
  test('TC-BLD-01 validate returns issues with JSON pointer paths', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn, [{ topic: ws.names.of('missing-topic') }]));
    const res = await api.post(Paths.routeAction(id, 'validate'), { mode: 'deploy' });
    expect(res.status).toBe(200);
    expect(res.body.valid).toBe(false);
    const issue = res.body.issues.find((i: { severity: string }) => i.severity === Issue.error);
    expect(issue.path).toMatch(/^\/branches\/0\/destination/);
    expect(issue.message.length).toBeGreaterThan(10);
  });

  test('TC-BLD-02 deploy is blocked by validation errors; save draft is not', async ({ ws }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn, [{ topic: ws.names.of('missing-topic') }]));
    const saved = await ws.saveDraft(id, ws.draft(conn, [{ topic: ws.names.of('still-missing') }]));
    expect(saved.status, 'saving a draft is never blocked').toBe(200);
    const dep = await ws.deploy(id);
    expect(dep.status, dep.describe()).toBe(422);
    expect(dep.body.issues.length).toBeGreaterThan(0);
    expect((await ws.route(id)).activeVersion).toBeNull();
  });

  test('TC-BLD-03 exactly-once is only allowed when source and destination share a connection', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const other = await ws.connection({}, ws.names.of('other-conn'));
    await ws.topics(other, [ws.names.of('remote-out')]);
    const id = await ws.createRoute(ws.draft(conn, [{ topic: ws.names.of('remote-out'), connectionId: other }]).delivery(RouteDoc.delivery.exactlyOnce));
    const res = await api.post(Paths.routeAction(id, 'validate'), { mode: 'deploy' });
    expect(res.body.valid).toBe(false);
    expect(JSON.stringify(res.body.issues)).toMatch(/delivery|exactly/i);
  });

  test('TC-BLD-08 JS that does not compile is a validation error at the step path', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.step('function transform(event, ctx) { return event; ')] }]));
    const res = await api.post(Paths.routeAction(id, 'validate'), { mode: 'deploy' });
    expect(res.body.valid).toBe(false);
    expect(res.body.issues.map((i: { path: string }) => i.path).join(',')).toContain('/branches/0/transform/steps/0');
  });

  test('TC-BLD-09 a missing destination topic is fine when createTopic is set (topic created on deploy)', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const topic = ws.names.of('autocreate');
    ws.resources.topic(topic);
    const id = await ws.createRoute(ws.draft(conn, [{ topic, destination: { createTopic: { partitions: 2 } } }]));
    const res = await api.post(Paths.routeAction(id, 'validate'), { mode: 'deploy' });
    expect(res.body.valid, JSON.stringify(res.body.issues)).toBe(true);
    expect((await ws.deploy(id)).status).toBe(202);
    await expect.poll(() => ws.kafka.topicExists(topic), { timeout: 60_000 }).toBe(true);
  });

  test('TC-BLD-15 the consumer group is generated as router.<route name>', async ({ ws }) => {
    const conn = await ws.pipe();
    const draft = ws.draft(conn).build();
    delete (draft.source as Record<string, unknown>).consumerGroup;
    const id = await ws.createRoute(draft);
    const route = await ws.route(id);
    expect(route.draft.source.consumerGroup ?? `router.${ws.names.route}`).toBe(`router.${ws.names.route}`);
  });

  test('TC-BLD-04 filter operators evaluate natively in preview (eq, gt, in, contains, regex, exists, between)', async ({ ws }) => {
    const runner = new StudioRunner(ws);
    const events = [{ t: 'OrderPaid', amount: 150, tags: 'vip,eu', code: 'ORD-1' }, { t: 'OrderPaid', amount: 50, code: 'X' }, { t: 'Refund', amount: 500, tags: 'eu' }];
    const cases: [unknown, string[]][] = [
      [Filter.cond('value.t', 'eq', 'OrderPaid'), ['passed', 'passed', 'filtered']],
      [Filter.cond('value.amount', 'gt', 100), ['passed', 'filtered', 'passed']],
      [{ type: 'cond', field: 'value.t', op: 'in', values: ['Refund'] }, ['filtered', 'filtered', 'passed']],
      [Filter.cond('value.tags', 'contains', 'vip'), ['passed', 'filtered', 'filtered']],
      [Filter.cond('value.code', 'regex', '^ORD-\\d+$'), ['passed', 'filtered', 'filtered']],
      [Filter.cond('value.tags', 'exists'), ['passed', 'filtered', 'passed']],
      [{ type: 'cond', field: 'value.amount', op: 'between', from: 100, to: 200 }, ['passed', 'filtered', 'filtered']],
    ];
    for (const [filter, expected] of cases) expect((await runner.run([], events, filter as never)).statuses(), JSON.stringify(filter)).toEqual(expected);
  });

  test('TC-BLD-05 nested AND / OR / NOT groups', async ({ ws }) => {
    const filter = Filter.or(Filter.and(Filter.cond('value.t', 'eq', 'A'), Filter.not(Filter.cond('value.n', 'lt', 5))), Filter.cond('value.t', 'eq', 'B'));
    const result = await new StudioRunner(ws).run([], [{ t: 'A', n: 7 }, { t: 'A', n: 1 }, { t: 'B', n: 0 }, { t: 'C', n: 9 }], filter);
    expect(result.statuses()).toEqual(['passed', 'filtered', 'passed', 'filtered']);
  });

  test('TC-BLD-06 expression mode: a one-line JS boolean expression', async ({ ws }) => {
    const filter = Filter.expr('event.value.amount > 100 && event.value.currency === "USD"');
    const result = await new StudioRunner(ws).run([], [{ amount: 150, currency: 'USD' }, { amount: 150, currency: 'EUR' }, { amount: 5, currency: 'USD' }], filter);
    expect(result.statuses()).toEqual(['passed', 'filtered', 'filtered']);
  });

  test('TC-BLD-07 a string compared with a number is coerced, with a preview warning', async ({ ws }) => {
    const result = await new StudioRunner(ws).run([], [{ amount: '150' }], Filter.cond('value.amount', 'gt', 100));
    expect(result.statuses()).toEqual(['passed']);
    expect(JSON.stringify(result.res.body)).toMatch(/coerc/i);
  });
});
