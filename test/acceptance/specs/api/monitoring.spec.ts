import { expect, test } from '../../lib/Fixtures';
import { Paths } from '../../lib/Platform';

/** Dashboard, route metrics, flow map, onboarding, alert rules (UI spec §4, §10.1, §11.4, §12; SysDesign §12). */
const Monitoring = { ranges: ['15m', '1h', '24h', '7d'], webhook: 'https://127.0.0.1:18999/acc-hook', webhookSecret: 'https://hooks.example.invalid/acc-secret-token' } as const;

test.describe('monitoring', () => {
  test('TC-MON-01 dashboard health cards count routes by state', async ({ ws, api }) => {
    const before = (await api.get('/api/v1/dashboard/summary')).body;
    const conn = await ws.pipe();
    await ws.createRoute(ws.draft(conn));
    const after = await api.get('/api/v1/dashboard/summary');
    expect(after.status, after.describe()).toBe(200);
    expect(after.body.routes.draft).toBe(before.routes.draft + 1);
    expect(after.body).toHaveProperty('thresholds.totalLagWarn');
    expect(typeof after.body.metricsAvailable).toBe('boolean');
  });

  test('TC-MON-03 recent activity lists who created, deployed and paused the route', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn));
    await ws.pause(id);
    await expect
      .poll(async () => ((await api.get('/api/v1/activity?limit=50')).body.items ?? []).filter((a: { routeId: string }) => a.routeId === id).map((a: { verb: string }) => a.verb), { timeout: 15_000 })
      .toEqual(expect.arrayContaining(['created', 'deployed', 'paused']));
  });

  test('TC-MON-04 route metrics answer every range and reject unknown ones', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn));
    for (const range of Monitoring.ranges) {
      const res = await api.get(`${Paths.route(id)}/metrics?range=${range}`);
      expect(res.status, `${range}: ${res.describe()}`).toBe(200);
      expect(res.body).toMatchObject({ range, pipeline: expect.any(Array), series: expect.objectContaining({ eventsIn: expect.any(Array), latencyP95: expect.any(Array) }) });
    }
    expect((await api.get(`${Paths.route(id)}/metrics?range=2y`)).status).toBeGreaterThanOrEqual(400);
  });

  test('TC-MON-05 the flow map draws a deployed route as an edge between its topics', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn));
    const map = await api.get('/api/v1/flow-map');
    expect(map.status).toBe(200);
    const edge = map.body.edges.find((e: { routeId: string }) => e.routeId === id);
    expect(edge).toBeTruthy();
    const labels = map.body.nodes.filter((n: { id: string }) => [edge.from, edge.to].includes(n.id)).map((n: { label: string }) => n.label);
    expect(labels).toEqual(expect.arrayContaining([ws.names.inTopic, ws.names.outTopic]));
  });

  test('TC-MON-06 the first-run checklist is derived from data', async ({ api }) => {
    const res = await api.get('/api/v1/onboarding');
    expect(res.status).toBe(200);
    expect(res.body.steps.map((s: { id: string }) => s.id)).toEqual(['connection', 'route', 'deploy']);
    expect(res.body.steps.every((s: { href: string }) => s.href.startsWith('/'))).toBe(true);
  });

  test('TC-MON-07 Try with demo data creates a demo connection with an order-events topic', async ({ api, ws }) => {
    const res = await api.post('/api/v1/onboarding/demo', {});
    expect([201, 200, 409], res.describe()).toContain(res.status);
    if (res.status === 201) ws.resources.onCleanup(() => api.delete('/api/v1/onboarding/demo'));
    const conns = await api.get('/api/v1/connections?limit=200');
    const demo = conns.body.items.find((c: { kind: string }) => c.kind === 'demo');
    expect(demo).toBeTruthy();
    const topics = await api.get(Paths.connectionTopics(demo.id));
    expect(topics.body.items.map((t: { name: string }) => t.name)).toContain('order-events');
  });

  test('TC-MON-08 alert rule CRUD; webhook URLs are write-only', async ({ ws, api, names }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    const created = await api.post(Paths.alertRules, { routeId: id, name: names.of('lag'), kind: 'lag_above', threshold: 1000, windowMinutes: 5, channels: [{ type: 'webhook', secret: Monitoring.webhookSecret }, { type: 'email', target: 'ops@example.com' }] });
    expect(created.status, created.describe()).toBe(201);
    ws.resources.onCleanup(() => api.delete(`${Paths.alertRules}/${created.body.id}`));
    expect(created.text).not.toContain('acc-secret-token');
    expect(created.body.channels).toEqual(expect.arrayContaining([expect.objectContaining({ type: 'webhook', secretSet: true })]));
    const listed = await api.get(`${Paths.alertRules}?routeId=${id}`);
    expect(listed.text).not.toContain('acc-secret-token');
    const patched = await api.patch(`${Paths.alertRules}/${created.body.id}`, { threshold: 2000 }, created.etag || `"${created.body.version}"`);
    expect(patched.status, patched.describe()).toBe(200);
    expect(patched.body.threshold).toBe(2000);
    expect(patched.body.channels.find((c: { type: string }) => c.type === 'webhook').secretSet, 'omitted secrets are kept').toBe(true);
    expect((await api.delete(`${Paths.alertRules}/${created.body.id}`)).status).toBe(204);
  });

  test('TC-MON-09 alert rule validation: webhook must be https, window within 1..1440 minutes', async ({ ws, api, names }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    const base = { routeId: id, name: names.of('bad'), kind: 'throughput_zero', threshold: 0, windowMinutes: 5 };
    expect((await api.post(Paths.alertRules, { ...base, channels: [{ type: 'webhook', secret: 'ftp://nope' }] })).status).toBe(422);
    expect((await api.post(Paths.alertRules, { ...base, windowMinutes: 0 })).status).toBe(422);
  });

  test('TC-MON-14 test notification reports a result per channel', async ({ ws, api, names }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    const created = await api.post(Paths.alertRules, { routeId: id, name: names.of('test'), kind: 'error_rate_above', threshold: 5, windowMinutes: 5, channels: [{ type: 'webhook', secret: Monitoring.webhook }] });
    ws.resources.onCleanup(() => api.delete(`${Paths.alertRules}/${created.body.id}`));
    const res = await api.post(`${Paths.alertRules}/${created.body.id}/test`, {});
    expect(res.status, res.describe()).toBe(200);
    expect(JSON.stringify(res.body)).toMatch(/"channel":"webhook"/);
    expect(JSON.stringify(res.body)).toMatch(/"ok":false/);
  });

  test('TC-MON-16 route runtime reports observed state and replica versions', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn));
    const res = await api.get(`${Paths.route(id)}/runtime`);
    expect(res.status).toBe(200);
    expect(res.body).toMatchObject({ observedState: 'running', runningVersionNo: 1 });
    expect(res.body.replicas.length).toBeGreaterThanOrEqual(1);
  });

  test('TC-MON-17 the bell feed returns alert history with an unread count', async ({ api }) => {
    const res = await api.get('/api/v1/alerts?limit=20');
    expect(res.status, res.describe()).toBe(200);
    expect(res.body).toHaveProperty('items');
    expect(JSON.stringify(Object.keys(res.body))).toMatch(/unread/i);
  });
});
