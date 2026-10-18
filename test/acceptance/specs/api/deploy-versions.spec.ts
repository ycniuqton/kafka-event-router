import { Http } from '../../lib/Api';
import { expect, test } from '../../lib/Fixtures';
import { Paths, type Workspace } from '../../lib/Platform';
import { Filter, JsSteps, Steps } from '../../lib/RouteDraft';

/** Deploy, versions, diff, rollback, pause/resume and YAML import/export (UI spec §7.4, §11.1; SysDesign §9.4). */
const Deploy = { autoRollbackWindowMs: 150_000 } as const;

/** A route with two deployed versions (v1 tag 1, v2 tag 2 + filter). */
async function twoVersions(ws: Workspace): Promise<{ id: string; conn: string }> {
  const conn = await ws.pipe();
  const id = await ws.createRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.tagVersion(1)] }]));
  expect((await ws.deploy(id, 'first')).status).toBe(202);
  await ws.saveDraft(id, ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.tagVersion(2)], filter: Filter.cond('value.eventType', 'eq', 'OrderPaid') }]));
  expect((await ws.deploy(id, 'second')).status).toBe(202);
  return { id, conn };
}

test.describe('deploy and versions', () => {
  test('TC-DEP-01 deploy creates immutable v1, the route runs and the deployment turns active after the safety window', async ({ ws }) => {
    test.setTimeout(240_000);
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    const dep = await ws.deploy(id, 'first deploy');
    expect(dep.status, dep.describe()).toBe(202);
    expect(dep.body.version).toMatchObject({ no: 1, origin: 'deploy', changeNote: 'first deploy' });
    expect(dep.body.version.configHash).toBeTruthy();
    await ws.waitObserved(id, 'running');
    await expect.poll(async () => (await ws.route(id)).lastDeployment?.status, { timeout: Deploy.autoRollbackWindowMs, intervals: [5_000] }).toBe('active');
  });

  test('TC-DEP-02 a second deploy creates v2; versions are listed newest first with author and note', async ({ ws, api }) => {
    const { id } = await twoVersions(ws);
    const versions = await api.get(`${Paths.route(id)}/versions`);
    expect(versions.body.items.map((v: { no: number }) => v.no)).toEqual([2, 1]);
    expect(versions.body.items[0]).toMatchObject({ changeNote: 'second', active: true });
    expect(versions.body.items[1]).toMatchObject({ changeNote: 'first', active: false });
    expect(versions.body.items[0].author?.name).toBeTruthy();
  });

  test('TC-DEP-03 the diff of two versions is readable per section', async ({ ws, api }) => {
    const { id } = await twoVersions(ws);
    const diff = await api.get(`${Paths.route(id)}/versions/1/diff/2`);
    expect(diff.status, diff.describe()).toBe(200);
    const sections = Object.fromEntries(diff.body.sections.map((s: { section: string; lines: { text: string }[] }) => [s.section, s.lines.map((l) => l.text).join('\n')]));
    expect(sections.filter).toMatch(/eventType/);
    expect(sections.filter).not.toMatch(/^\{/);
    expect(sections.transform).toBeTruthy();
  });

  test('TC-DEP-04 rollback redeploys v1 as v3; history is kept', async ({ ws, api }) => {
    const { id } = await twoVersions(ws);
    const rb = await api.post(Paths.routeAction(id, 'rollback'), { version: 1, changeNote: 'back to v1' });
    expect(rb.status, rb.describe()).toBe(202);
    expect(rb.body.version).toMatchObject({ no: 3, origin: 'rollback', basedOn: 1 });
    const v1 = (await api.get(`${Paths.route(id)}/versions/1`)).body.config;
    const v3 = (await api.get(`${Paths.route(id)}/versions/3`)).body.config;
    expect(v3.branches).toEqual(v1.branches);
    expect((await api.get(`${Paths.route(id)}/versions`)).body.items.map((v: { no: number }) => v.no)).toEqual([3, 2, 1]);
    expect((await ws.route(id)).activeVersion.no).toBe(3);
  });

  test('TC-DEP-05 diff active ↔ draft shows the undeployed changes', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    await ws.deploy(id);
    await ws.saveDraft(id, ws.draft(conn, [{ topic: ws.names.outTopic, steps: [Steps.rename('value.a', 'value.b')] }]));
    expect((await ws.route(id)).hasUndeployedDraft).toBe(true);
    const diff = await api.get(`${Paths.route(id)}/versions/active/diff/draft`);
    expect(JSON.stringify(diff.body.sections)).toMatch(/rename|value\.a/i);
  });

  test('TC-DEP-06 deploy with a stale If-Match is refused (412)', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    const stale = (await api.get(Paths.route(id))).etag;
    await ws.saveDraft(id, ws.draft(conn));
    expect((await api.post(Paths.routeAction(id, 'deploy'), {}, stale)).status).toBe(412);
  });

  test('TC-DEP-07 YAML export carries connection names, never ids or secrets', async ({ ws, api }) => {
    await ws.pipe();
    const conn = await ws.connection({ security: { mode: 'sasl_plain', username: 'acc' }, secrets: { password: 'acc-export-secret' } }, ws.names.of('sasl'));
    const id = await ws.createRoute(ws.draft(conn));
    const res = await api.get(`${Paths.route(id)}/export?format=yaml&version=draft`);
    expect(res.status, res.describe()).toBe(200);
    expect(res.headers['content-disposition']).toContain(`${ws.names.route}.yaml`);
    expect(res.text).toContain(`connectionId: ${ws.names.of('sasl')}`);
    expect(res.text).not.toContain(conn);
    expect(res.text).not.toContain('acc-export-secret');
  });

  test('TC-DEP-08 import YAML: dry run previews, import creates a draft and never deploys', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    const yaml = (await api.get(`${Paths.route(id)}/export?format=yaml&version=draft`)).text;
    const copy = ws.names.of('imported');
    const doc = yaml.replace(`name: ${ws.names.route}\n`, `name: ${copy}\n`);
    const dry = await api.post('/api/v1/routes/import?dryRun=true', { content: doc, format: 'yaml' });
    expect(dry.status, dry.describe()).toBe(200);
    expect(dry.body.results[0]).toMatchObject({ name: copy, action: 'created' });
    expect((await api.get(`${Paths.routes}?q=${copy}`)).body.items).toHaveLength(0);
    const real = await api.post('/api/v1/routes/import', { content: doc, format: 'yaml' });
    const routeId = ws.resources.route(real.body.results[0].routeId);
    const route = await ws.route(routeId);
    expect(route.status).toBe('draft');
    expect(route.activeVersion).toBeNull();
  });

  test('TC-DEP-09 import in upsert mode replaces the draft of an existing route', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    const yaml = (await api.get(`${Paths.route(id)}/export?format=yaml&version=draft`)).text;
    const skipped = await api.post('/api/v1/routes/import', { content: yaml, format: 'yaml' });
    expect(skipped.body.results[0].action).toBe('skipped_exists');
    const changed = yaml.replace(`topic: ${ws.names.outTopic}`, `topic: ${ws.names.inTopic}`);
    const upsert = await api.post('/api/v1/routes/import?mode=upsert', { content: changed, format: 'yaml' });
    expect(upsert.body.results[0].action).toBe('draft_updated');
    expect((await ws.route(id)).draft.branches[0].destination.topic).toBe(ws.names.inTopic);
  });

  test('TC-DEP-10 pause / resume change the desired state; drafts cannot be paused', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const draftOnly = await ws.createRoute(ws.draft(conn, undefined, ws.names.of('draft-only')));
    expect((await ws.pause(draftOnly)).status).toBe(409);
    const id = await ws.runningRoute(ws.draft(conn));
    const paused = await ws.pause(id);
    expect(paused.body.desiredState).toBe('paused');
    await ws.waitObserved(id, 'paused');
    expect((await ws.resume(id)).body.desiredState).toBe('running');
    await ws.waitObserved(id, 'running');
    const deployments = await api.get(`${Paths.route(id)}/deployments`);
    expect(deployments.body.items).toHaveLength(1);
  });

  test('TC-DEP-15 routes PATCH without If-Match is refused (428)', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    expect((await api.call(Http.Patch, Paths.route(id), { description: 'x' })).status).toBe(428);
  });
});
