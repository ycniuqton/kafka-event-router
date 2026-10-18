import type { AccApi } from '../../lib/Api';
import { expect, test } from '../../lib/Fixtures';
import { Paths, type Workspace } from '../../lib/Platform';
import { Steps } from '../../lib/RouteDraft';
import { StudioRunner } from '../../lib/Studio';

/** Saved test cases (UI spec §8.6) and the transform library / lookup tables (§8.7). */
const Library = { transforms: '/api/v1/transforms', tables: '/api/v1/lookup-tables' } as const;

/** Creates a library object of `kind` and removes it at the end. */
async function libraryObject(ws: Workspace, api: AccApi, kind: string, body: Record<string, unknown>): Promise<any> {
  const res = await api.post(kind, { teamId: await api.teamId(), description: 'acceptance', changeNote: 'v1', ...body });
  expect(res.status, res.describe()).toBe(201);
  ws.resources.onCleanup(() => api.delete(`${kind}/${res.body.id}`));
  return res.body;
}

test.describe('saved test cases', () => {
  test('TC-TS-24 a pinned test case passes, and fails with a diff after an incompatible edit', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const steps = [Steps.rename('value.cust_id', 'value.customerId')];
    const draft = ws.draft(conn, [{ topic: ws.names.outTopic, steps }]).build();
    const id = await ws.createRoute(draft);
    const input = { key: 'k1', value: { cust_id: 7 } };
    const created = await api.post(`${Paths.route(id)}/test-cases`, { branchId: 'main', name: 'rename keeps id', inputEvent: input, expectedOutcome: 'passed', expectedOutput: { key: 'k1', value: { customerId: 7 }, headers: {} } });
    expect(created.status, created.describe()).toBe(201);
    const pass = await api.post(`${Paths.route(id)}/test-cases/run`, { config: draft });
    expect(pass.status, pass.describe()).toBe(200);
    expect(pass.body, JSON.stringify(pass.body)).toMatchObject({ passed: 1, failed: 0 });
    const broken = ws.draft(conn, [{ topic: ws.names.outTopic, steps: [Steps.rename('value.cust_id', 'value.customer')] }]).build();
    const fail = await api.post(`${Paths.route(id)}/test-cases/run`, { config: broken });
    expect(fail.body.failed, fail.describe()).toBe(1);
    expect(fail.body.results[0].result).toBe('fail');
    expect(fail.body.results[0].diff.length).toBeGreaterThan(0);
  });

  test('TC-TS-25 a failing test case blocks deploy', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [Steps.setConstant('value.v', 1)] }]));
    await api.post(`${Paths.route(id)}/test-cases`, { branchId: 'main', name: 'v is 2', inputEvent: { value: { a: 1 } }, expectedOutcome: 'passed', expectedOutput: { key: null, value: { a: 1, v: 2 }, headers: {} } });
    const dep = await ws.deploy(id);
    expect(dep.status, dep.describe()).toBe(422);
    expect(JSON.stringify(dep.body.issues)).toMatch(/test/i);
  });
});

test.describe('transform library', () => {
  test('TC-TS-26 a library transform is used as one pinned step and behaves like its steps', async ({ ws, api }) => {
    const name = ws.names.of('mask-pii');
    await libraryObject(ws, api, Library.transforms, { name, steps: [Steps.mask('value.email', 'email'), Steps.remove('value.ssn')] });
    const out = await new StudioRunner(ws).one([{ kind: 'library', ref: name, version: 1 }], { email: 'john@mail.com', ssn: '123', id: 1 });
    expect(out).toEqual({ email: 'j***@mail.com', id: 1 });
  });

  test('TC-TS-27 a new library version shows as available but pinned routes do not change', async ({ ws, api }) => {
    const name = ws.names.of('lib-v');
    const lib = await libraryObject(ws, api, Library.transforms, { name, steps: [Steps.setConstant('value.lib', 1)] });
    const v2 = await api.post(`${Library.transforms}/${lib.id}/versions`, { steps: [Steps.setConstant('value.lib', 2)], changeNote: 'v2' });
    expect(v2.status, v2.describe()).toBeLessThan(300);
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [{ kind: 'library', ref: name, version: 1 }] }]));
    ws.resources.onRelease(() => ws.saveDraft(id, ws.draft(conn)));
    expect((await new StudioRunner(ws).one([{ kind: 'library', ref: name, version: 1 }], { a: 1 })).lib).toBe(1);
    const item = await api.get(`${Library.transforms}/${lib.id}`);
    expect(item.body.latestVersion).toBe(2);
    const usages = await api.get(`${Library.transforms}/${lib.id}/usages`);
    expect(JSON.stringify(usages.body)).toContain(id);
    const validation = await api.post(Paths.routeAction(id, 'validate'), { mode: 'draft' });
    expect(JSON.stringify(validation.body.issues)).toMatch(/update|newer|version 2/i);
  });

  test('TC-TS-28 a versioned lookup table is used by the lookup recipe', async ({ ws, api }) => {
    const name = ws.names.of('countries');
    await libraryObject(ws, api, Library.tables, { name, keyType: 'string', rows: [{ key: 'VN', value: 'Vietnam' }, { key: 'DE', value: 'Germany' }] });
    const out = await new StudioRunner(ws).one([{ kind: 'lookup', field: 'value.cc', to: 'value.country', ref: name, version: 1, defaultValue: 'Unknown' }], { cc: 'VN' });
    expect(out.country).toBe('Vietnam');
  });
});
