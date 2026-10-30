import { expect, test } from '../../lib/Fixtures';
import { FakeCrm, FakeCrmSpec, LookupSources } from '../../lib/FakeLookup';
import { Http } from '../../lib/Api';
import { Paths, type Workspace } from '../../lib/Platform';
import { AccTarget, Profile } from '../../lib/Target';

/** Lookup sources and the source-mode lookup step (connections 00001-lookup-sources, pipeline-engine 00002-lookups). */
const Lks = { pollMs: 60_000, auditAction: 'lookup_source.created', dlqTransient: 'lookup_unavailable', transientClass: 'transient' } as const;

/** The fake CRM runs in the test runner, so the platform must run on this machine. */
function localOnly(): void {
  test.skip(Profile.target !== AccTarget.Local, 'the fake lookup target runs on the test runner (local stack only)');
}

/** Creates an HTTP source on the fake CRM and registers its cleanup (after the routes that use it). */
async function crmSource(ws: Workspace, crm: FakeCrm, extra: Record<string, unknown> = {}): Promise<string> {
  const res = await ws.api.post(LookupSources.path, LookupSources.http(`${ws.names.of('lks')}`, await ws.api.teamId(), crm.baseUrl, extra));
  expect(res.status, res.describe()).toBe(201);
  ws.resources.onCleanup(() => ws.api.delete(LookupSources.item(res.body.id)));
  return res.body.id;
}

test.describe('lookup sources', () => {
  let crm: FakeCrm;
  test.beforeEach(async () => {
    localOnly();
    crm = await new FakeCrm().start();
  });
  test.afterEach(async () => crm?.stop());

  test('TC-LKS-01 create an HTTP source; the token is write-only and the create is audited without it', async ({ ws, api }) => {
    const id = await crmSource(ws, crm);
    const got = await api.get(LookupSources.item(id));
    expect(got.status, got.describe()).toBe(200);
    expect(got.body).toMatchObject({ kind: 'http', envTag: 'dev', usedBy: { count: 0 }, http: { baseUrl: crm.baseUrl, auth: { mode: 'bearer' } } });
    expect(got.body.secretFields).toContain('token');
    expect(got.text).not.toContain(FakeCrmSpec.token);
    const audit = await api.get(`${Paths.auditLog}?action=${Lks.auditAction}&limit=50`);
    expect(audit.text).toContain(id);
    expect(audit.text).not.toContain(FakeCrmSpec.token);
  });

  test('TC-LKS-02 invalid settings are rejected with a JSON Pointer to the field', async ({ ws, api }) => {
    const body = LookupSources.http(ws.names.of('lks'), await api.teamId(), 'ftp://nope');
    const res = await api.post(LookupSources.path, body);
    expect(res.status, res.describe()).toBe(422);
    expect(JSON.stringify(res.body)).toMatch(/\/http\/baseUrl/);
  });

  test('TC-LKS-03 the Test checklist passes on a reachable source and explains an auth failure', async ({ ws, api }) => {
    const id = await crmSource(ws, crm);
    const ok = await api.post(`${LookupSources.item(id)}/test`);
    expect(ok.status, ok.describe()).toBe(200);
    expect(ok.body.ok).toBe(true);
    expect(ok.body.steps.map((s: any) => s.step)).toEqual(['reachable', 'authenticated', 'sample']);
    expect((await api.get(LookupSources.item(id))).body.health.status).toBe(ok.body.health);
    const wrong = await crmSource(ws, crm, { name: ws.names.of('lksbad'), secrets: { token: 'wrong' } });
    const bad = await api.post(`${LookupSources.item(wrong)}/test`);
    expect(bad.body.ok).toBe(false);
    const failed = bad.body.steps.find((s: any) => s.state === 'failed');
    expect(failed?.step).toBe('authenticated');
    expect(failed?.message, 'plain-language reason').toBeTruthy();
  });

  test('TC-LKS-04 an unsaved draft can be tested and nothing is stored', async ({ ws, api }) => {
    const name = ws.names.of('lksdraft');
    const res = await api.post(`${LookupSources.path}/test`, LookupSources.http(name, await api.teamId(), crm.baseUrl));
    expect(res.status, res.describe()).toBe(200);
    expect(res.body.ok).toBe(true);
    const list = await api.get(`${LookupSources.path}?q=${encodeURIComponent(name)}`);
    expect((list.body.items ?? []).filter((s: any) => s.name === name)).toHaveLength(0);
  });

  test('TC-LKS-05 PATCH needs If-Match and keeps the stored secret when secrets are omitted', async ({ ws, api }) => {
    const id = await crmSource(ws, crm);
    const noMatch = await api.patch(LookupSources.item(id), { description: 'changed' });
    expect(noMatch.status, noMatch.describe()).toBe(428);
    const patched = await api.withEtag(Http.Patch, LookupSources.item(id), { description: 'changed' });
    expect(patched.status, patched.describe()).toBe(200);
    expect(patched.body.secretFields).toContain('token');
    expect((await api.post(`${LookupSources.item(id)}/test`)).body.ok, 'the kept token still authenticates').toBe(true);
  });

  test('TC-LKS-06 a source used by a route cannot be deleted; an unknown sourceId fails route validation', async ({ ws, api }) => {
    const id = await crmSource(ws, crm);
    const conn = await ws.pipe();
    const route = await ws.createRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [LookupSources.step(id)] }]));
    const del = await api.delete(LookupSources.item(id));
    expect(del.status, del.describe()).toBe(409);
    expect((await api.get(LookupSources.item(id))).body.usedBy.routes.map((u: any) => u.id)).toContain(route);
    const unknown = await ws.saveDraft(route, ws.draft(conn, [{ topic: ws.names.outTopic, steps: [LookupSources.step('00000000-0000-0000-0000-000000000000')] }]));
    const validation = await api.post(`${Paths.route(route)}/validate`, {});
    expect([unknown.status, validation.status].some((s) => s === 422) || JSON.stringify(validation.body).includes('sourceId'), `${unknown.describe()} ${validation.describe()}`).toBeTruthy();
  });

  test('TC-LKS-07 a preview run calls the live source and enriches the sample', async ({ ws, api }) => {
    const id = await crmSource(ws, crm);
    const conn = await ws.pipe();
    const draft = ws.draft(conn, [{ topic: ws.names.outTopic, steps: [LookupSources.step(id)] }]).build();
    const session = await ws.uploadSession([{ key: 'k', value: { customerId: 'c1' } }]);
    const run = await ws.previewRun(session, draft);
    expect(run.status, run.describe()).toBe(200);
    expect(run.text).toContain('gold');
    expect(crm.requests.some((r) => r.startsWith(`${FakeCrmSpec.customersPath}c1`))).toBe(true);
  });

  test('TC-LKS-08 a running route enriches events; an outage dead-letters them as lookup_unavailable (transient)', async ({ ws, api }) => {
    const id = await crmSource(ws, crm);
    const conn = await ws.pipe();
    const route = await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [LookupSources.step(id)] }]));
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'a', value: { customerId: 'c1' } }, { key: 'b', value: { customerId: 'c2' } }]);
    const out = await ws.kafka.consume(ws.names.outTopic, 2);
    expect(out.map((r) => r.value.customer.tier).sort()).toEqual(['gold', 'silver']);
    crm.outage();
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'c', value: { customerId: 'c1' } }]);
    let items: any[] = [];
    await expect.poll(async () => (items = (await api.get(`${Paths.dlq(route)}?limit=10`)).body?.items ?? []).length, { timeout: Lks.pollMs, intervals: [2_000] }).toBeGreaterThanOrEqual(1);
    expect(items[0]).toMatchObject({ errorType: Lks.dlqTransient, errorClass: Lks.transientClass, step: 'crm' });
  });
});
