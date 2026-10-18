import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { AccApi, Http } from '../../lib/Api';
import { expect, Role, test } from '../../lib/Fixtures';
import { AuthPaths } from '../../lib/Login';
import { Paths } from '../../lib/Platform';
import { JsSteps } from '../../lib/RouteDraft';
import { WorkspaceSettings } from '../../lib/Settings';
import { StudioRunner } from '../../lib/Studio';
import { AccTarget, Profile } from '../../lib/Target';

/** Audit log (UI spec §11.3), settings, and security: secrets, sandbox, auth (SysDesign §7.5, §11). */
const Security = {
  password: 'acc-never-returned-pw',
  workerLog: join(process.cwd(), '..', '..', '.local', 'logs', 'worker.log'),
  check: '/api/v1/sandbox/check',
} as const;

/** Audit queries. */
async function auditFor(api: AccApi, objectId: string): Promise<any[]> {
  return (await api.get(`${Paths.auditLog}?objectId=${objectId}&limit=100`)).body.items ?? [];
}

test.describe('audit log', () => {
  test('TC-AUD-01 every mutation is audited with actor, time and IP', async ({ ws, api }) => {
    const conn = await ws.connection();
    const entries = await auditFor(api, conn);
    expect(entries.map((e) => e.action)).toContain('connection.created');
    expect(entries[0]).toMatchObject({ actor: expect.objectContaining({ name: expect.any(String) }), ip: expect.any(String), objectType: 'connection' });
    expect(Date.parse(entries[0].at)).not.toBeNaN();
  });

  test('TC-AUD-02 filter by object and action', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn));
    await ws.pause(id);
    const paused = (await api.get(`${Paths.auditLog}?objectId=${id}&action=route.paused`)).body.items;
    expect(paused).toHaveLength(1);
    expect(paused[0].objectLabel).toBe(ws.names.route);
  });

  test('TC-AUD-03 viewing raw samples of a sensitive connection is audited as sensitive access', async ({ ws, api }) => {
    const conn = await ws.connection({ sensitive: true });
    await ws.topics(conn, [ws.names.inTopic]);
    await api.post(Paths.preview.samples, { connectionId: conn, topics: [ws.names.inTopic], mode: 'latest', count: 1 });
    await expect.poll(async () => (await api.get(`${Paths.auditLog}?sensitive=true&q=${ws.names.connection}&limit=20`)).body.items?.length ?? 0, { timeout: 10_000 }).toBeGreaterThan(0);
  });

  test('TC-AUD-04 only workspace admins can read the audit log', async ({ users }) => {
    const editor = await users.as(Role.Editor);
    expect((await editor.get(Paths.auditLog)).status).toBe(403);
  });

  test('TC-AUD-05 the audit log is read-only through the API', async ({ api }) => {
    const first = (await api.get(`${Paths.auditLog}?limit=1`)).body.items[0];
    for (const method of [Http.Patch, Http.Delete, Http.Put]) {
      const res = await api.call(method, `${Paths.auditLog}/${first.id}`, method === Http.Delete ? undefined : {});
      expect(res.status, method).toBeGreaterThanOrEqual(400);
    }
    expect((await api.post(Paths.auditLog, {})).status).toBeGreaterThanOrEqual(400);
  });

  test('TC-AUD-06 export the filtered audit log as JSONL; the export itself is audited', async ({ ws, api }) => {
    const conn = await ws.connection();
    const res = await api.get(`/api/v1/audit-log/export?format=jsonl&objectId=${conn}`);
    expect(res.status, res.describe()).toBe(200);
    const lines = res.text.trim().split('\n').map((l) => JSON.parse(l));
    expect(lines.map((l) => l.action)).toContain('connection.created');
    await expect.poll(async () => (await api.get(`${Paths.auditLog}?action=audit.exported&limit=5`)).body.items?.length ?? 0, { timeout: 10_000 }).toBeGreaterThan(0);
  });
});

test.describe('settings', () => {
  test('TC-SET-01 workspace settings are readable by any member; AI assist is a workspace toggle', async ({ api, users }) => {
    const res = await api.get('/api/v1/workspace-settings');
    expect(res.status).toBe(200);
    expect(typeof res.body.aiAssistEnabled).toBe('boolean');
    const viewer = await users.as(Role.Viewer);
    expect((await viewer.get('/api/v1/workspace-settings')).status).toBe(200);
  });

  test('TC-SET-02 changing settings needs settings.manage and is audited', async ({ api, users }) => {
    const settings = new WorkspaceSettings(api);
    const current = (await settings.current()).body.defaultRequireProdApproval;
    await settings.with({ defaultRequireProdApproval: !current }, async () => undefined);
    const entries = (await api.get(`${Paths.auditLog}?action=settings.updated&limit=5`)).body.items;
    expect(entries.length).toBeGreaterThan(0);
    const editor = await users.as(Role.Editor);
    const { ifMatch } = await settings.current();
    expect((await editor.patch('/api/v1/workspace-settings', { aiAssistEnabled: true }, ifMatch)).status).toBe(403);
  });

  test('TC-SET-03 teams: create, rename; delete is blocked while the team owns objects', async ({ ws, api, names }) => {
    const team = await api.post('/api/v1/teams', { name: names.of('team') });
    expect(team.status, team.describe()).toBe(201);
    expect(team.body.requireProdApproval).toBe(true);
    const teamPath = Paths.team(team.body.id);
    ws.resources.onCleanup(() => api.withEtag(Http.Delete, teamPath));
    const renamed = await api.withEtag(Http.Patch, teamPath, { name: names.of('team2') });
    expect(renamed.status, renamed.describe()).toBe(200);
    const member = await api.put(`/api/v1/role-bindings/${(await api.me()).id}/${team.body.id}`, { role: Role.Admin });
    expect(member.ok, member.describe()).toBeTruthy();
    const conn = await ws.connection({ owner: { teamId: team.body.id } });
    const blocked = await api.withEtag(Http.Delete, teamPath);
    expect(blocked.status, blocked.describe()).toBe(409);
    expect((await api.delete(Paths.connection(conn))).status).toBe(204);
    const deleted = await api.withEtag(Http.Delete, teamPath);
    expect(deleted.status, deleted.describe()).toBe(204);
  });

  test('TC-SET-04 pre-invite a user by email with a team role', async ({ ws, api, names }) => {
    const email = names.email('invited');
    const res = await api.post('/api/v1/users/invitations', { email, teamId: await api.teamId(), role: 'viewer' });
    expect(res.status, res.describe()).toBe(201);
    ws.resources.onCleanup(() => api.withEtag(Http.Patch, Paths.user(res.body.id), { disabled: true }));
    const users = await api.get(`/api/v1/users?q=${encodeURIComponent(email)}`);
    expect(JSON.stringify(users.body)).toContain('invited');
  });

  test('TC-SET-05 API tokens: the secret is shown once and never listed', async ({ api, names }) => {
    const created = await api.post('/api/v1/auth/tokens', { name: names.of('token'), scopes: ['read'], expires_in_days: 1 });
    expect(created.body.token).toMatch(/^ert_/);
    try {
      const list = await api.get('/api/v1/auth/tokens');
      expect(list.text).not.toContain(created.body.token);
      expect(JSON.stringify(list.body.items)).toContain(created.body.prefix);
    } finally {
      await api.delete(`/api/v1/auth/tokens/${created.body.id}`);
    }
  });
});

test.describe('security · secrets', () => {
  test('TC-SEC-01 a connection password is never returned (get, list, export, audit)', async ({ ws, api }) => {
    const conn = await ws.connection({ security: { mode: 'sasl_scram_512', username: 'acc' }, secrets: { password: Security.password } });
    const route = await ws.createRoute(ws.draft(conn));
    const responses = [
      await api.get(Paths.connection(conn)),
      await api.get(`${Paths.connections}?limit=200`),
      await api.get(`${Paths.route(route)}/export?format=yaml&version=draft`),
      await api.get(`${Paths.auditLog}?objectId=${conn}`),
    ];
    for (const res of responses) expect(res.text, res.text.slice(0, 80)).not.toContain(Security.password);
    expect(responses[0].body.security.secretFields.password).toBe('set');
  });

  test('TC-SEC-02 payloads and secrets never appear in worker logs (local only)', async ({ ws }) => {
    test.skip(Profile.target !== AccTarget.Local || !existsSync(Security.workerLog), 'reads the local worker log');
    const conn = await ws.pipe();
    await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.step(JsSteps.throwOnBad)] }]));
    const marker = `acc-payload-marker-${Date.now()}`;
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'k', value: { marker, bad: true, id: 1 } }, { key: 'k2', value: { marker } }]);
    await ws.kafka.consume(ws.names.outTopic, 1);
    expect(readFileSync(Security.workerLog, 'utf8')).not.toContain(marker);
  });
});

test.describe('security · sandbox', () => {
  test('TC-SEC-03 no network: fetch / XMLHttpRequest / require do not exist', async ({ ws }) => {
    const code = 'function transform(e) { e.value.t = [typeof fetch, typeof XMLHttpRequest, typeof require, typeof process, typeof WebSocket].join(","); return e; }';
    const out = await new StudioRunner(ws).one([JsSteps.step(code)], { a: 1 });
    expect(out.t).toBe('undefined,undefined,undefined,undefined,undefined');
  });

  test('TC-SEC-04 an infinite loop hits the per-event time limit and is reported, not hung', async ({ ws }) => {
    const started = Date.now();
    const result = await new StudioRunner(ws).run([JsSteps.step(JsSteps.infiniteLoop)], [{ a: 1 }]);
    expect(result.statuses()).toEqual(['error']);
    expect(JSON.stringify(result.errors())).toMatch(/timeout|time limit|exceeded/i);
    expect(Date.now() - started).toBeLessThan(10_000);
  });

  test('TC-SEC-05 a memory bomb hits the memory limit', async ({ ws }) => {
    const code = 'function transform(e) { const a = []; while (true) { a.push(new Array(1e6).fill(1)); } }';
    const result = await new StudioRunner(ws).run([JsSteps.step(code)], [{ a: 1 }]);
    expect(result.statuses()).toEqual(['error']);
    expect(JSON.stringify(result.errors())).toMatch(/memory|timeout|out of/i);
  });

  test('TC-SEC-06 no host access: globalThis has no credentials, environment or file system', async ({ ws }) => {
    const code = 'function transform(e) { e.value.keys = Object.getOwnPropertyNames(globalThis).filter(k => /env|secret|fs|file|net|os|std/i.test(k)); return e; }';
    expect((await new StudioRunner(ws).one([JsSteps.step(code)], { a: 1 })).keys).toEqual([]);
  });

  test('TC-SEC-07 the check API reports syntax errors with line and column', async ({ api }) => {
    const res = await api.post(Security.check, { code: 'function transform(event, ctx) {\n  return event.;\n}' });
    expect(res.status, res.describe()).toBe(200);
    expect(res.body.diagnostics[0]).toMatchObject({ severity: 'error', line: 2 });
    expect((await api.post(Security.check, { code: JsSteps.identity })).body.diagnostics).toEqual([]);
  });

  test('TC-SEC-08 state does not leak between events (fresh context per call)', async ({ ws }) => {
    const code = 'function transform(e) { globalThis.counter = (globalThis.counter || 0) + 1; Object.prototype.polluted = "x"; e.value.c = globalThis.counter; return e; }';
    const result = await new StudioRunner(ws).run([JsSteps.step(code)], [{ a: 1 }, { a: 2 }, { a: 3 }]);
    expect(result.values.map((v) => v.c)).toEqual([1, 1, 1]);
  });

  test('TC-SEC-14 route JS limits are clamped to the operator maxima', async ({ ws, api }) => {
    const info = await api.get('/api/v1/sandbox/runtime-info');
    expect(info.body.limits.defaults).toMatchObject({ jsTimeoutMs: 50, jsMemoryMb: 64 });
    expect(info.body.safetyRules.length).toBeGreaterThan(0);
    const max = info.body.limits.maxima.jsTimeoutMs as number;
    const conn = await ws.pipe();
    const draft = ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.step(JsSteps.infiniteLoop)] }]).limits({ jsTimeoutMs: 5000 }).build();
    const session = await ws.uploadSession([{ value: { a: 1 } }]);
    const started = Date.now();
    const res = await ws.previewRun(session, draft);
    expect(res.body.results[0].status).toBe('error');
    expect(Date.now() - started, `5000 ms is clamped to the ${max} ms maximum`).toBeLessThan(max + 2_500);
  });
});

test.describe('security · authentication', () => {
  test('TC-SEC-09 unauthenticated API calls get 401', async ({ playwright }) => {
    const anon = await playwright.request.newContext({ baseURL: Profile.baseURL });
    try {
      for (const path of [Paths.routes, Paths.connections, AuthPaths.me, Paths.auditLog]) expect((await anon.get(path)).status(), path).toBe(401);
    } finally {
      await anon.dispose();
    }
  });

  test('TC-SEC-10 cookie-authenticated writes without the CSRF token are refused', async ({ api, names }) => {
    const res = await api.request.post(Paths.connections, { data: { name: names.of('csrf'), envTag: 'dev', bootstrapServers: [Profile.platformBootstrap], security: { mode: 'plaintext' } }, failOnStatusCode: false });
    if (res.status() === 201) await api.delete(Paths.connection((await res.json()).id));
    expect(res.status()).toBe(403);
  });

  test('TC-SEC-11 dev login is refused outside local', async ({ playwright }) => {
    test.skip(Profile.target === AccTarget.Local, 'dev login is the local sign-in method');
    const anon = await playwright.request.newContext({ baseURL: Profile.baseURL });
    try {
      expect((await anon.post(AuthPaths.devLogin, { data: {} })).status()).toBeGreaterThanOrEqual(400);
    } finally {
      await anon.dispose();
    }
  });
});
