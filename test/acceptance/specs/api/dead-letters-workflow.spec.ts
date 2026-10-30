import { expect, test, Role } from '../../lib/Fixtures';
import { FakeCrm, LookupSources } from '../../lib/FakeLookup';
import { Paths, type Workspace } from '../../lib/Platform';
import { JsSteps } from '../../lib/RouteDraft';
import { AccTarget, Profile } from '../../lib/Target';
import type { AccApi } from '../../lib/Api';

/** Dead-letter workflow: classes, cause groups, edit / dry-run / bulk replay, 4-eyes, auto-retry (dead-letters 00001-workflow). */
const Dlw = {
  pollMs: 60_000,
  jobMs: 90_000,
  alertMs: 120_000,
  dataClass: 'data',
  stepError: 'step_error',
  routerKind: 'router_caused_dead_letters',
  editedAudit: 'dlq.edited_replayed',
  pending: 'pending_approval',
  jobs: '/api/v1/dlq/jobs',
} as const;

/** A route whose JS step throws on `bad` events (message carries the event id), with its dead letters. */
class WorkflowRoute {
  id = '';
  conn = '';

  constructor(private readonly ws: Workspace) {}

  async start(bad: number, conn?: string, approver?: AccApi): Promise<this> {
    this.conn = conn ?? (await this.ws.pipe());
    const draft = this.ws.draft(this.conn, [{ topic: this.ws.names.outTopic, steps: [JsSteps.step(JsSteps.throwOnBad)] }]);
    this.id = approver ? await this.approvedRoute(draft, approver) : await this.ws.runningRoute(draft);
    await this.ws.kafka.produce(this.ws.names.inTopic, Array.from({ length: bad }, (_, i) => ({ key: `bad-${i}`, value: { id: 1000 + i, bad: true } })));
    await this.list(bad);
    return this;
  }

  /** Production deploys wait for an approval: `approver` approves, then the route runs. */
  private async approvedRoute(draft: ReturnType<Workspace['draft']>, approver: AccApi): Promise<string> {
    const id = await this.ws.createRoute(draft);
    const dep = await this.ws.deploy(id);
    expect(dep.status, dep.describe()).toBe(202);
    if (dep.body.deployment?.status === Dlw.pending) {
      const path = `/api/v1/approvals/${dep.body.deployment.id}`;
      const detail = await approver.get(path);
      const ok = await approver.post(`${path}/approve`, { note: 'acc' }, detail.etag || `"${detail.body.approval.rowVersion}"`);
      expect(ok.status, ok.describe()).toBe(200);
    }
    await this.ws.waitObserved(id, 'running');
    return id;
  }

  get dlq(): string {
    return Paths.dlq(this.id);
  }

  async list(count: number, query = ''): Promise<any[]> {
    let items: any[] = [];
    await expect
      .poll(async () => (items = (await this.ws.api.get(`${this.dlq}?limit=100${query}`)).body?.items ?? []).length, { timeout: Dlw.pollMs, intervals: [2_000] })
      .toBeGreaterThanOrEqual(count);
    return items;
  }

  async groups(api: AccApi = this.ws.api): Promise<any> {
    const res = await api.get(`${this.dlq}/groups`);
    expect(res.status, res.describe()).toBe(200);
    return res.body;
  }

  /** Value edit that removes the `bad` flag (the record then passes the unchanged route). */
  static fixedEdit(id: number): Record<string, unknown> {
    return { value: { encoding: 'json', data: { id, bad: false, edited: true } } };
  }
}

test.describe('dead-letter workflow', () => {
  test('TC-DLW-01 every dead letter carries its error class; a user script error is bad data', async ({ ws, api }) => {
    const route = await new WorkflowRoute(ws).start(1);
    const [item] = await route.list(1);
    expect(item).toMatchObject({ errorType: Dlw.stepError, errorClass: Dlw.dataClass });
    const detail = await api.get(`${route.dlq}/${encodeURIComponent(item.id)}`);
    expect(detail.body.errorClass).toBe(Dlw.dataClass);
  });

  test('TC-DLW-02 failures with the same cause form one group with a normalized pattern; the group drills into the list', async ({ ws }) => {
    const route = await new WorkflowRoute(ws).start(3);
    const page = await route.groups();
    expect(page.items).toHaveLength(1);
    const [group] = page.items;
    expect(group).toMatchObject({ count: 3, errorType: Dlw.stepError, errorClass: Dlw.dataClass, step: 'js' });
    expect(group.pattern).toContain('<n>');
    expect(group.pattern).not.toMatch(/100\d/);
    expect(page.byClass[Dlw.dataClass]).toBe(3);
    expect(await route.list(3, `&fingerprint=${encodeURIComponent(group.fingerprint)}`)).toHaveLength(3);
  });

  test('TC-DLW-03 the global cause view lists the group of the route', async ({ ws, api }) => {
    const route = await new WorkflowRoute(ws).start(2);
    await expect.poll(async () => ((await api.get(`/api/v1/dlq/groups?routeId=${route.id}`)).body?.items ?? []).filter((g: any) => g.routeId === route.id).length, { timeout: 30_000 }).toBe(1);
  });

  test('TC-DLW-04 a dry run shows the outputs and produces nothing', async ({ ws, api }) => {
    const route = await new WorkflowRoute(ws).start(1);
    const [item] = await route.list(1);
    const dry = await api.post(`${route.dlq}/${encodeURIComponent(item.id)}/replay`, { dryRun: true, edit: WorkflowRoute.fixedEdit(1000) });
    expect(dry.status, dry.describe()).toBe(200);
    expect(dry.body).toMatchObject({ dryRun: true, activeVersionNo: 1, result: { outcome: 'would_send' } });
    expect(dry.body.result.outputs[0].topic).toBe(ws.names.outTopic);
    expect(dry.body.diff.map((d: any) => d.path)).toEqual(expect.arrayContaining(['/value/bad']));
    expect(await ws.kafka.drain(ws.names.outTopic), 'nothing was produced').toHaveLength(0);
    expect(await route.list(1)).toHaveLength(1);
  });

  test('TC-DLW-05 an unedited dry run fails again with the same error', async ({ ws, api }) => {
    const route = await new WorkflowRoute(ws).start(1);
    const [item] = await route.list(1);
    const dry = await api.post(`${route.dlq}/${encodeURIComponent(item.id)}/replay`, { dryRun: true });
    expect(dry.status, dry.describe()).toBe(200);
    expect(dry.body.result.outcome).toBe('failed');
    expect(dry.body.result.message).toContain('bad event');
  });

  test('TC-DLW-06 edit then replay delivers the edited event, resolves it and audits the diff without values', async ({ ws, api }) => {
    const route = await new WorkflowRoute(ws).start(1);
    const [item] = await route.list(1);
    const res = await api.post(`${route.dlq}/${encodeURIComponent(item.id)}/replay`, { edit: WorkflowRoute.fixedEdit(1000), note: 'fixed flag' });
    expect(res.status, res.describe()).toBe(200);
    const [out] = await ws.kafka.consume(ws.names.outTopic, 1);
    expect(out.value).toMatchObject({ id: 1000, edited: true, bad: false });
    await expect.poll(async () => ((await api.get(`${route.dlq}?limit=10`)).body?.items ?? []).length, { timeout: 20_000 }).toBe(0);
    const audit = await api.get(`${Paths.auditLog}?action=${Dlw.editedAudit}&limit=20`);
    expect(audit.text).toContain('/value/bad');
    expect(audit.text).not.toContain('"edited":true');
  });

  test('TC-DLW-07 an edit that is not valid JSON for a JSON source is rejected', async ({ ws, api }) => {
    const route = await new WorkflowRoute(ws).start(1);
    const [item] = await route.list(1);
    const res = await api.post(`${route.dlq}/${encodeURIComponent(item.id)}/replay`, { dryRun: true, edit: { value: { encoding: 'utf8', data: '{not json' } } });
    expect(res.status, res.describe()).toBe(400);
    expect(res.body.path).toBe('/edit/value');
  });

  test('TC-DLW-08 a group dry run samples the group and counts outcomes', async ({ ws, api }) => {
    const route = await new WorkflowRoute(ws).start(3);
    const [group] = (await route.groups()).items;
    const res = await api.post(`${route.dlq}/dry-run`, { match: { fingerprint: group.fingerprint } });
    expect(res.status, res.describe()).toBe(200);
    expect(res.body).toMatchObject({ sampled: 3, total: 3 });
    expect(res.body.counts.failed).toBe(3);
  });

  test('TC-DLW-09 bulk replay of a group runs as a rate-limited job after a fix', async ({ ws, api }) => {
    const route = await new WorkflowRoute(ws).start(4);
    const [group] = (await route.groups()).items;
    await ws.saveDraft(route.id, ws.draft(route.conn, [{ topic: ws.names.outTopic, steps: [JsSteps.step(JsSteps.identity)] }]));
    expect((await ws.deploy(route.id, 'fix')).status).toBe(202);
    await ws.waitRunningVersion(route.id, 2);
    const job = await api.post(`${route.dlq}/jobs`, { action: 'retry', match: { fingerprint: group.fingerprint }, ratePerSecond: 2 });
    expect(job.status, job.describe()).toBe(202);
    await expect.poll(async () => (await api.get(`${Dlw.jobs}/${job.body.jobId}`)).body?.status, { timeout: Dlw.jobMs, intervals: [2_000] }).toBe('succeeded');
    const done = (await api.get(`${Dlw.jobs}/${job.body.jobId}`)).body;
    expect(done).toMatchObject({ total: 4, processed: 4 });
    expect(await ws.kafka.consume(ws.names.outTopic, 4)).toHaveLength(4);
  });

  test('TC-DLW-10 viewers cannot dry-run or replay', async ({ ws, users }) => {
    const route = await new WorkflowRoute(ws).start(1);
    const [item] = await route.list(1);
    const viewer = await users.as(Role.Viewer);
    expect((await viewer.post(`${route.dlq}/${encodeURIComponent(item.id)}/replay`, { dryRun: true })).status).toBe(403);
    expect((await viewer.post(`${route.dlq}/dry-run`, { ids: [item.id] })).status).toBe(403);
  });

  test('TC-DLW-11 a replay into production needs a second person (4-eyes)', async ({ ws, users }) => {
    const editor = await users.as(Role.Editor);
    const approver = await users.as(Role.Approver);
    const prod = await ws.connection({ envTag: 'prod' });
    await ws.topics(prod, [ws.names.inTopic, ws.names.outTopic]);
    ws.resources.topic(ws.names.dlqTopic());
    ws.resources.group(ws.names.group());
    const route = await new WorkflowRoute(ws).start(1, prod, approver);
    const [item] = await route.list(1);
    const sync = await editor.post(`${route.dlq}/retry`, { ids: [item.id] });
    expect(sync.status, sync.describe()).toBe(409);
    const req = await editor.post(`${route.dlq}/${encodeURIComponent(item.id)}/replay`, { edit: WorkflowRoute.fixedEdit(1000), note: 'please' });
    expect(req.status, req.describe()).toBe(202);
    expect(req.body.status).toBe(Dlw.pending);
    const jobId = req.body.job.jobId;
    expect((await editor.post(`${Dlw.jobs}/${jobId}/approve`, {})).status, 'the requester cannot approve').toBeGreaterThanOrEqual(403);
    const ok = await approver.post(`${Dlw.jobs}/${jobId}/approve`, { note: 'lgtm' });
    expect(ok.status, ok.describe()).toBe(200);
    const [out] = await ws.kafka.consume(ws.names.outTopic, 1, Dlw.jobMs);
    expect(out.value.edited).toBe(true);
  });

  test('TC-DLW-12 every route gets the router-caused dead-letters alert rule by default', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const route = await ws.createRoute(ws.draft(conn));
    await expect
      .poll(async () => ((await api.get(`${Paths.alertRules}?routeId=${route}`)).body?.items ?? []).filter((r: any) => r.kind === Dlw.routerKind && r.enabled).length, { timeout: Dlw.alertMs, intervals: [5_000] })
      .toBe(1);
  });
});

test.describe('auto-retry of transient errors', () => {
  test('TC-DLW-13 a transient lookup outage heals through retry topics; exhausted retries park with the attempt count', async ({ ws, api }) => {
    test.skip(Profile.target !== AccTarget.Local, 'the fake lookup target runs on the test runner (local stack only)');
    const crm = await new FakeCrm().start();
    try {
      const src = await api.post(LookupSources.path, LookupSources.http(ws.names.of('lks'), await api.teamId(), crm.baseUrl));
      expect(src.status, src.describe()).toBe(201);
      ws.resources.onCleanup(() => api.delete(LookupSources.item(src.body.id)));
      const conn = await ws.pipe();
      const draft = ws.draft(conn, [{ topic: ws.names.outTopic, steps: [LookupSources.step(src.body.id)] }]).onError('dlq', { retry: { enabled: true, maxAttempts: 2, baseDelayMs: 500, maxDelayMs: 1000 } });
      for (const n of [1, 2]) ws.resources.topic(`router.retry.${ws.names.route}.${n}`);
      const route = await ws.runningRoute(draft);
      crm.failNext(1);
      await ws.kafka.produce(ws.names.inTopic, [{ key: 'heal', value: { customerId: 'c1' } }]);
      const [healed] = await ws.kafka.consume(ws.names.outTopic, 1);
      expect(healed.value.customer.tier, 'recovered on a retry attempt').toBe('gold');
      crm.outage();
      await ws.kafka.produce(ws.names.inTopic, [{ key: 'park', value: { customerId: 'c2' } }]);
      let items: any[] = [];
      await expect.poll(async () => (items = (await api.get(`${Paths.dlq(route)}?limit=10`)).body?.items ?? []).length, { timeout: Dlw.pollMs, intervals: [2_000] }).toBe(1);
      expect(items[0]).toMatchObject({ errorType: 'lookup_unavailable', errorClass: 'transient', attempts: 2 });
    } finally {
      await crm.stop();
    }
  });
});
