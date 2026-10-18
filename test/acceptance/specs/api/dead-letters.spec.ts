import { expect, test } from '../../lib/Fixtures';
import { Paths, type Workspace } from '../../lib/Platform';
import { JsSteps } from '../../lib/RouteDraft';

/** Dead letters: browse, detail, retry, discard/restore, export, Open in Studio, jobs (UI spec §10.3, SysDesign §8.4). */
const Dlq = { pollMs: 60_000 } as const;

/** A running route whose JS step throws on `bad` events, with `bad` dead letters already in its DLQ. */
class FailingRoute {
  id = '';
  conn = '';

  constructor(private readonly ws: Workspace) {}

  async start(bad: number, good = 1): Promise<this> {
    this.conn = await this.ws.pipe();
    this.id = await this.ws.runningRoute(this.ws.draft(this.conn, [{ topic: this.ws.names.outTopic, steps: [JsSteps.step(JsSteps.throwOnBad)] }]));
    const events = [...Array.from({ length: bad }, (_, i) => ({ key: `bad-${i}`, value: { id: i, bad: true } })), ...Array.from({ length: good }, (_, i) => ({ key: `ok-${i}`, value: { id: 100 + i } }))];
    await this.ws.kafka.produce(this.ws.names.inTopic, events);
    await this.ws.kafka.consume(this.ws.names.outTopic, good);
    await this.list(bad);
    return this;
  }

  /** Waits until at least `count` unresolved dead letters are listed; returns them. */
  async list(count: number, query = ''): Promise<any[]> {
    let items: any[] = [];
    await expect
      .poll(async () => (items = (await this.ws.api.get(`${Paths.dlq(this.id)}?limit=100${query}`)).body?.items ?? []).length, { timeout: Dlq.pollMs, intervals: [2_000] })
      .toBeGreaterThanOrEqual(count);
    return items;
  }

  async count(): Promise<number> {
    return ((await this.ws.api.get(`${Paths.dlq(this.id)}?limit=100`)).body?.items ?? []).length;
  }

  /** Deploys a fixed version that no longer throws. */
  async fix(): Promise<void> {
    await this.ws.saveDraft(this.id, this.ws.draft(this.conn, [{ topic: this.ws.names.outTopic, steps: [JsSteps.step(JsSteps.identity)] }]));
    expect((await this.ws.deploy(this.id, 'fix')).status).toBe(202);
  }
}

test.describe('dead letters', () => {
  test('TC-DLQ-01 the list shows time, error type, message, step and source offset', async ({ ws }) => {
    const route = await new FailingRoute(ws).start(2);
    const [item] = await route.list(2);
    expect(item).toMatchObject({ routeId: route.id, errorType: expect.stringMatching(/step|runtime/), versionNo: 1 });
    expect(item.message).toContain('bad event');
    expect(item.source).toMatchObject({ topic: ws.names.inTopic });
    expect(Date.parse(item.failedAt)).not.toBeNaN();
  });

  test('TC-DLQ-02 the detail has the original event, the failing step and the JS stack', async ({ ws, api }) => {
    const route = await new FailingRoute(ws).start(1);
    const [item] = await route.list(1);
    const detail = await api.get(`${Paths.dlq(route.id)}/${encodeURIComponent(item.id)}`);
    expect(detail.status, detail.describe()).toBe(200);
    expect(detail.body.value.data).toEqual({ id: 0, bad: true });
    expect(detail.body.key.data).toBe('bad-0');
    expect(detail.body.stepLabel).toMatch(/JavaScript|js/i);
    expect(detail.body.stack).toMatch(/transform|step\.js/);
  });

  test('TC-DLQ-03 retry after a fix delivers the event and hides it from the list', async ({ ws, api }) => {
    const route = await new FailingRoute(ws).start(2);
    await route.fix();
    await ws.waitRunningVersion(route.id, 2);
    const items = await route.list(2);
    const retry = await api.post(`${Paths.dlq(route.id)}/retry`, { ids: [items[0].id] });
    expect(retry.status, retry.describe()).toBe(200);
    expect(retry.body.activeVersionNo).toBe(2);
    expect(JSON.stringify(retry.body.results)).toMatch(/sent|produced|resolved/);
    const out = await ws.kafka.consume(ws.names.outTopic, 2);
    expect(out.filter((r) => r.key?.startsWith('bad-')), 'the retried event reached the destination').toHaveLength(1);
    await expect.poll(() => route.count(), { timeout: 20_000 }).toBe(1);
  });

  test('TC-DLQ-04 retry without a fix fails again and the event stays', async ({ ws, api }) => {
    const route = await new FailingRoute(ws).start(1);
    const [item] = await route.list(1);
    const retry = await api.post(`${Paths.dlq(route.id)}/retry`, { ids: [item.id] });
    expect(retry.status, retry.describe()).toBe(200);
    expect(JSON.stringify(retry.body.results)).toMatch(/fail/i);
    expect(await route.count()).toBe(1);
  });

  test('TC-DLQ-05 discard hides an event; restore brings it back', async ({ ws, api }) => {
    const route = await new FailingRoute(ws).start(2);
    const items = await route.list(2);
    const discard = await api.post(`${Paths.dlq(route.id)}/discard`, { ids: [items[0].id] });
    expect(discard.status, discard.describe()).toBe(200);
    expect(discard.body.discarded).toBe(1);
    await expect.poll(() => route.count(), { timeout: 20_000 }).toBe(1);
    const restore = await api.post(`${Paths.dlq(route.id)}/restore`, { ids: [items[0].id] });
    expect(restore.body.restored, restore.describe()).toBe(1);
    await expect.poll(() => route.count(), { timeout: 20_000 }).toBe(2);
  });

  test('TC-DLQ-06 export selected dead letters as a JSON array of details', async ({ ws, api }) => {
    const route = await new FailingRoute(ws).start(2);
    const items = await route.list(2);
    const res = await api.get(`${Paths.dlq(route.id)}/export?ids=${items.map((i) => encodeURIComponent(i.id)).join(',')}&format=json`);
    expect(res.status, res.describe()).toBe(200);
    expect(res.headers['content-disposition']).toContain('attachment');
    expect(res.body).toHaveLength(2);
    expect(res.body[0]).toHaveProperty('value');
  });

  test('TC-DLQ-07 Open in Studio loads the failing event as a sample session', async ({ ws, api }) => {
    const route = await new FailingRoute(ws).start(1);
    const [item] = await route.list(1);
    const link = await api.post(`${Paths.dlq(route.id)}/${encodeURIComponent(item.id)}/studio`, {});
    expect(link.status, link.describe()).toBeLessThan(300);
    expect(link.body.builderUrl).toMatch(/step=transform/);
    const events = await api.get(`/api/v1/preview/sessions/${link.body.sessionId}/events`);
    expect(JSON.stringify(events.body.events)).toContain('"bad":true');
  });

  test('TC-DLQ-08 the summary counts unresolved dead letters by error type', async ({ ws, api }) => {
    const route = await new FailingRoute(ws).start(3);
    await expect.poll(async () => (await api.get(`${Paths.dlq(route.id)}/summary`)).body?.unresolved, { timeout: 30_000 }).toBe(3);
    const summary = (await api.get(`${Paths.dlq(route.id)}/summary`)).body;
    expect(Object.values(summary.byErrorType).reduce((a: number, b) => a + Number(b), 0)).toBe(3);
  });

  test('TC-DLQ-09 a background job retries every dead letter matching a filter', async ({ ws, api }) => {
    const route = await new FailingRoute(ws).start(3);
    await route.fix();
    await ws.waitRunningVersion(route.id, 2);
    const job = await api.post(`${Paths.dlq(route.id)}/jobs`, { action: 'retry', match: {} });
    expect(job.status, job.describe()).toBeLessThan(300);
    await expect.poll(async () => (await api.get(`/api/v1/dlq/jobs/${job.body.jobId}`)).body?.status, { timeout: 60_000, intervals: [2_000] }).toBe('succeeded');
    await expect.poll(() => route.count(), { timeout: 20_000 }).toBe(0);
    expect(await ws.kafka.consume(ws.names.outTopic, 4)).toHaveLength(4);
  });

  test('TC-DLQ-10 the global dead-letter view lists events across routes', async ({ ws, api }) => {
    const route = await new FailingRoute(ws).start(1);
    await expect.poll(async () => JSON.stringify((await api.get('/api/v1/dlq?limit=200')).body?.items ?? []).includes(route.id), { timeout: 30_000 }).toBe(true);
  });

  test('TC-DLQ-12 filter dead letters by text', async ({ ws }) => {
    const route = await new FailingRoute(ws).start(2);
    const filtered = await route.list(1, '&q=bad%20event%201');
    expect(filtered.every((i) => i.message.includes('bad event 1'))).toBe(true);
  });
});
