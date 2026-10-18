import { AccApi, Http } from '../../lib/Api';
import { expect, Role, test } from '../../lib/Fixtures';
import { FixtureTeams, Paths, type Workspace } from '../../lib/Platform';
import { Profile } from '../../lib/Target';

/** Roles, RBAC, approvals and masking (UI spec §11.2, access-control requirements). */
const Approval = { pending: 'pending_approval', rejected: 'rejected', cancelled: 'cancelled', path: (id: string) => `/api/v1/approvals/${id}` } as const;

/** A production connection + topics owned by the admin, and a route draft created by `author` on it. */
async function prodRoute(ws: Workspace, author: AccApi): Promise<{ conn: string; route: string }> {
  const conn = await ws.connection({ envTag: 'prod' });
  await ws.topics(conn, [ws.names.inTopic, ws.names.outTopic]);
  ws.resources.group(ws.names.group());
  const draft = ws.draft(conn).build();
  const res = await author.post(Paths.routes, { name: ws.names.route, teamId: await author.teamId(), draft });
  expect(res.status, res.describe()).toBe(201);
  return { conn, route: ws.resources.route(res.body.id) };
}

/** Deploy as `who`; returns the deployment of the 202 answer. */
async function deployAs(who: AccApi, routeId: string, note = 'acc'): Promise<any> {
  const etag = (await who.get(Paths.route(routeId))).etag;
  const res = await who.post(Paths.routeAction(routeId, 'deploy'), { changeNote: note }, etag);
  expect(res.status, res.describe()).toBe(202);
  return res.body.deployment;
}

async function decide(who: AccApi, approvalId: string, action: 'approve' | 'reject', note?: string): Promise<any> {
  const detail = await who.get(Approval.path(approvalId));
  const ifMatch = detail.etag || `"${detail.body.approval.rowVersion}"`;
  return who.post(`${Approval.path(approvalId)}/${action}`, note === undefined ? {} : { note }, ifMatch);
}

test.describe('roles and RBAC', () => {
  test('TC-APR-01 a viewer cannot create routes (403 naming the missing permission)', async ({ ws, users }) => {
    const viewer = await users.as(Role.Viewer);
    const conn = await ws.pipe();
    const res = await viewer.post(Paths.routes, { name: ws.names.route, teamId: await viewer.teamId(), draft: ws.draft(conn).build() });
    if (res.status === 201) ws.resources.route(res.body.id);
    expect(res.status).toBe(403);
    expect(res.text).toMatch(/route\.write|editor/i);
  });

  test('TC-APR-02 only admins manage connections: viewer and editor get 403', async ({ users, names }) => {
    for (const role of [Role.Viewer, Role.Editor]) {
      const who = await users.as(role);
      const res = await who.post(Paths.connections, { name: names.of(role), envTag: 'dev', bootstrapServers: [Profile.platformBootstrap], security: { mode: 'plaintext' }, owner: { teamId: await who.teamId() } });
      expect(res.status, `${role}: ${res.describe()}`).toBe(403);
    }
  });

  test('TC-APR-03 an editor creates and deploys a route to a non-production connection directly', async ({ ws, users }) => {
    const editor = await users.as(Role.Editor);
    const conn = await ws.pipe();
    const res = await editor.post(Paths.routes, { name: ws.names.route, teamId: await editor.teamId(), draft: ws.draft(conn).build() });
    expect(res.status, res.describe()).toBe(201);
    ws.resources.route(res.body.id);
    const dep = await deployAs(editor, res.body.id);
    expect(dep.status).not.toBe(Approval.pending);
  });

  test('TC-APR-12 a viewer can read routes, history and metrics', async ({ ws, users }) => {
    const viewer = await users.as(Role.Viewer);
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    await ws.deploy(id);
    for (const path of [Paths.route(id), `${Paths.route(id)}/versions`, `${Paths.route(id)}/metrics?range=1h`, Paths.connections]) {
      expect((await viewer.get(path)).status, path).toBe(200);
    }
    expect((await viewer.post(Paths.routeAction(id, 'pause'), {})).status).toBe(403);
  });

  test('TC-APR-13 /auth/me embeds the effective permissions of each role', async ({ users }) => {
    const viewer = await (await users.as(Role.Viewer)).me();
    const access = Object.values(viewer.access.teams)[0];
    expect(access.role).toBe('viewer');
    expect(access.permissions).toContain('route.read');
    expect(access.permissions).not.toContain('route.write');
    expect(viewer.access.workspaceAdmin).toBe(false);
  });

  test('TC-APR-11 sample masking: viewers always masked, editors only on sensitive connections', async ({ ws, users }) => {
    const viewer = await users.as(Role.Viewer);
    const editor = await users.as(Role.Editor);
    const plain = await ws.pipe();
    const sensitive = await ws.connection({ sensitive: true }, ws.names.of('sensitive'));
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'k', value: { email: 'john@mail.com', n: 1 } }]);
    const load = (who: AccApi, conn: string) => who.post(Paths.preview.samples, { connectionId: conn, topics: [ws.names.inTopic], mode: 'latest', count: 5 });
    const cases: [AccApi, string, boolean][] = [[viewer, plain, true], [editor, plain, false], [editor, sensitive, true]];
    for (const [who, conn, masked] of cases) {
      const res = await load(who, conn);
      expect(res.ok, res.describe()).toBeTruthy();
      expect(res.body.session.masked, `${conn === sensitive ? 'sensitive' : 'plain'} masked=${masked}`).toBe(masked);
      expect(JSON.stringify(res.body.events).includes('john@mail.com')).toBe(!masked);
    }
  });

  test('TC-APR-17 a read-scoped API token can read but not write', async ({ api, names }) => {
    const created = await api.post('/api/v1/auth/tokens', { name: names.of('read-token'), scopes: ['read'], expires_in_days: 1 });
    expect(created.status, created.describe()).toBe(201);
    const token = await AccApi.withToken(Profile.baseURL, created.body.token);
    try {
      expect((await token.get(Paths.routes)).status).toBe(200);
      const write = await token.post(Paths.routes, { name: names.of('nope') });
      if (write.status === 201) await api.delete(Paths.route(write.body.id));
      expect(write.status, write.describe()).toBe(403);
    } finally {
      await token.dispose();
      await api.delete(`/api/v1/auth/tokens/${created.body.id}`);
    }
  });

  test('TC-APR-18 a revoked API token stops working immediately (401)', async ({ api, names }) => {
    const created = await api.post('/api/v1/auth/tokens', { name: names.of('revoke-token'), scopes: ['write'], expires_in_days: 1 });
    const token = await AccApi.withToken(Profile.baseURL, created.body.token);
    try {
      expect((await token.get(Paths.routes)).status).toBe(200);
      expect((await api.delete(`/api/v1/auth/tokens/${created.body.id}`)).status).toBe(204);
      expect((await token.get(Paths.routes)).status).toBe(401);
    } finally {
      await token.dispose();
    }
  });

  test('TC-APR-19 disabling a user revokes their sessions', async ({ users, api }) => {
    const viewer = await users.as(Role.Viewer);
    const me = await viewer.me();
    expect((await viewer.get(Paths.routes)).status).toBe(200);
    const res = await api.withEtag(Http.Patch, `/api/v1/users/${me.id}`, { disabled: true });
    expect(res.ok, res.describe()).toBeTruthy();
    await expect.poll(async () => (await viewer.get(Paths.routes)).status, { timeout: 10_000 }).toBe(401);
  });
});

test.describe('production approvals', () => {
  test('TC-APR-04 an editor deploy to production becomes an approval request', async ({ ws, users }) => {
    const editor = await users.as(Role.Editor);
    const { route } = await prodRoute(ws, editor);
    const dep = await deployAs(editor, route);
    expect(dep.status).toBe(Approval.pending);
    expect((await ws.route(route)).activeVersion).toBeNull();
  });

  test('TC-APR-05 the requester can never approve their own request; TC-APR-06 an approver activates it', async ({ ws, users }) => {
    const editor = await users.as(Role.Editor);
    const approver = await users.as(Role.Approver);
    const { route } = await prodRoute(ws, editor);
    const dep = await deployAs(editor, route);
    const self = await decide(editor, dep.id, 'approve');
    expect(self.status, self.describe()).toBe(403);
    const approved = await decide(approver, dep.id, 'approve', 'lgtm');
    expect(approved.status, approved.describe()).toBe(200);
    expect((await ws.route(route)).activeVersion?.no).toBe(1);
    const twice = await decide(approver, dep.id, 'approve');
    expect(twice.status, 'exactly one decision wins').toBe(409);
  });

  test('TC-APR-07 reject needs a note; the version number stays consumed', async ({ ws, users }) => {
    const editor = await users.as(Role.Editor);
    const approver = await users.as(Role.Approver);
    const { route } = await prodRoute(ws, editor);
    const dep = await deployAs(editor, route);
    expect((await decide(approver, dep.id, 'reject')).status).toBe(422);
    const rejected = await decide(approver, dep.id, 'reject', 'start from latest please');
    expect(rejected.status, rejected.describe()).toBe(200);
    expect(rejected.body.status).toBe(Approval.rejected);
    const next = await deployAs(editor, route);
    const detail = await editor.get(Approval.path(next.id));
    expect(detail.body.approval.versionNo).toBe(2);
  });

  test('TC-APR-08 the requester can withdraw a pending request; TC-APR-09 a newer request cancels the older one', async ({ ws, users }) => {
    const editor = await users.as(Role.Editor);
    const { route } = await prodRoute(ws, editor);
    const first = await deployAs(editor, route);
    const second = await deployAs(editor, route);
    const older = (await editor.get(Approval.path(first.id))).body.approval;
    expect(['cancelled', 'superseded'], 'the older request is no longer pending').toContain(older.status);
    expect((await decide(editor, first.id, 'approve')).status).toBeGreaterThanOrEqual(400);
    const withdrawn = await editor.post(`${Approval.path(second.id)}/withdraw`, {});
    expect(withdrawn.status, withdrawn.describe()).toBe(200);
    expect(withdrawn.body).toMatchObject({ status: Approval.cancelled, cancelReason: 'withdrawn' });
  });

  test('TC-APR-10 anyone involved can comment on a request', async ({ ws, users }) => {
    const editor = await users.as(Role.Editor);
    const approver = await users.as(Role.Approver);
    const { route } = await prodRoute(ws, editor);
    const dep = await deployAs(editor, route);
    const comment = await approver.post(`${Approval.path(dep.id)}/comments`, { body: 'why earliest?' });
    expect(comment.status, comment.describe()).toBe(201);
    const detail = await editor.get(Approval.path(dep.id));
    expect(detail.body.comments.map((c: { body: string }) => c.body)).toContain('why earliest?');
    expect(detail.body.canDecide).toBe(false);
    expect((await approver.get(Approval.path(dep.id))).body.canDecide).toBe(true);
  });
});

test.describe('teams and grants', () => {
  test('TC-APR-14 a route may only use connections owned by or granted to its team', async ({ ws, api }) => {
    const teamId = await new FixtureTeams(api).ensure(FixtureTeams.routeOwner);
    const conn = await ws.pipe();
    const route = await api.post(Paths.routes, { name: ws.names.route, teamId, draft: ws.draft(conn).build() });
    expect(route.status, route.describe()).toBe(201);
    ws.resources.route(route.body.id);
    const denied = await api.post(Paths.routeAction(route.body.id, 'validate'), { mode: 'deploy' });
    expect(JSON.stringify(denied.body.issues)).toMatch(/connection/i);
    const grant = await api.put(`${Paths.team(teamId)}/grants/${conn}`, {});
    expect(grant.ok, grant.describe()).toBeTruthy();
    ws.resources.onCleanup(() => api.delete(`${Paths.team(teamId)}/grants/${conn}`));
    const allowed = await api.post(Paths.routeAction(route.body.id, 'validate'), { mode: 'deploy' });
    expect(JSON.stringify(allowed.body.issues)).not.toMatch(/not granted|cannot use|grant/i);
  });
});
