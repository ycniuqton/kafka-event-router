import { Http } from '../../lib/Api';
import { expect, test } from '../../lib/Fixtures';
import { Paths } from '../../lib/Platform';
import { Profile } from '../../lib/Target';

/** Connections (UI spec §5, SysDesign §6 Connections API). */
const Conn = {
  unreachable: '127.0.0.1:1',
  steps: ['brokers_reachable', 'authenticated', 'list_topics'],
  writeHeader: 'x-router-test',
} as const;

/** Parses the NDJSON stream of a connection test. */
class TestStream {
  static parse(text: string): { steps: any[]; result: any } {
    const lines = text.split('\n').filter(Boolean).map((l) => JSON.parse(l));
    return { steps: lines.filter((l) => l.type === 'step'), result: lines.find((l) => l.type === 'result') };
  }

  static finalStates(steps: any[]): Record<string, string> {
    return Object.fromEntries(steps.map((s) => [s.step, s.state]));
  }
}

test.describe('connections', () => {
  test('TC-CONN-01 create a plaintext connection', async ({ ws, api }) => {
    const id = await ws.connection();
    const res = await api.get(Paths.connection(id));
    expect(res.status).toBe(200);
    expect(res.body).toMatchObject({ name: ws.names.connection, envTag: 'dev', bootstrapServers: [Profile.platformBootstrap], security: { mode: 'plaintext' }, usedBy: { count: 0 } });
    expect(res.etag).not.toBe('');
  });

  test('TC-CONN-02 test checklist passes with live ticks and marks the connection healthy', async ({ ws, api }) => {
    const id = await ws.connection();
    const res = await api.post(Paths.connectionTest(id), {});
    expect(res.status, res.describe()).toBe(200);
    const { steps, result } = TestStream.parse(res.text);
    const states = TestStream.finalStates(steps);
    for (const step of Conn.steps) expect(states[step], JSON.stringify(steps)).toBe('passed');
    expect(result).toMatchObject({ ok: true, health: 'healthy' });
    expect((await api.get(Paths.connection(id))).body.health.status).toBe('healthy');
  });

  test('TC-CONN-03 unreachable broker fails with a plain-language reason and hint', async ({ ws, api }) => {
    const id = await ws.connection({ bootstrapServers: [Conn.unreachable] });
    const res = await api.post(Paths.connectionTest(id), {});
    const { steps, result } = TestStream.parse(res.text);
    const failed = steps.find((s) => s.state === 'failed');
    expect(failed?.step).toBe('brokers_reachable');
    expect(failed?.reason).toBe('unreachable');
    expect(failed?.message?.length).toBeGreaterThan(10);
    expect(failed?.hint?.length).toBeGreaterThan(5);
    expect(result).toMatchObject({ ok: false, health: 'unreachable' });
  });

  test('TC-CONN-05 PATCH without secrets keeps the stored password', async ({ ws, api }) => {
    const id = await ws.connection({ security: { mode: 'sasl_plain', username: 'acc-user' }, secrets: { password: 'acc-pw-keep' } });
    const patched = await api.withEtag(Http.Patch, Paths.connection(id), { description: 'patched by acceptance' });
    expect(patched.status, patched.describe()).toBe(200);
    expect(patched.body.description).toBe('patched by acceptance');
    expect(patched.body.security.secretFields?.password).toBe('set');
  });

  test('TC-CONN-06 SASL without a username is rejected with the failing field path', async ({ ws, api }) => {
    const res = await api.post(Paths.connections, { name: ws.names.of('nouser'), envTag: 'dev', bootstrapServers: [Profile.platformBootstrap], security: { mode: 'sasl_plain' }, secrets: { password: 'x' } });
    if (res.status === 201) ws.resources.connection(res.body.id);
    expect(res.status, res.describe()).toBe(422);
    expect(JSON.stringify(res.body)).toMatch(/username/);
  });

  test('TC-CONN-07 delete is blocked while a route uses the connection', async ({ ws, api }) => {
    const conn = await ws.pipe();
    const route = await ws.createRoute(ws.draft(conn));
    const res = await api.delete(Paths.connection(conn));
    expect(res.status, res.describe()).toBe(409);
    expect(res.body.routes?.map((r: { id: string }) => r.id)).toContain(route);
    expect((await api.get(Paths.connection(conn))).body.usedBy.count).toBe(1);
  });

  test('TC-CONN-08 browse topics lists partitions of created topics', async ({ ws, api }) => {
    const conn = await ws.connection();
    await ws.topics(conn, [ws.names.inTopic], 3);
    const res = await api.get(`${Paths.connectionTopics(conn)}?q=${ws.names.inTopic}`);
    expect(res.status).toBe(200);
    expect(res.body.items).toEqual([expect.objectContaining({ name: ws.names.inTopic, partitions: 3 })]);
  });

  test('TC-CONN-09 topic creation: 201, duplicate 409, forbidden when the connection does not allow it', async ({ ws, api }) => {
    const conn = await ws.connection();
    await ws.topics(conn, [ws.names.inTopic]);
    const dup = await api.post(Paths.connectionTopics(conn), { name: ws.names.inTopic, partitions: 1 });
    expect(dup.status).toBe(409);
    const locked = await ws.connection({ allowTopicCreate: false }, ws.names.of('locked'));
    const denied = await api.post(Paths.connectionTopics(locked), { name: ws.names.of('denied'), partitions: 1 });
    expect(denied.status, denied.describe()).toBe(403);
  });

  test('TC-CONN-10 raw client properties outside the allow-list are rejected', async ({ ws, api }) => {
    const bad = await api.post(Paths.connections, { name: ws.names.of('props'), envTag: 'dev', bootstrapServers: [Profile.platformBootstrap], security: { mode: 'plaintext' }, properties: { 'sasl.jaas.config': 'x' } });
    if (bad.status === 201) ws.resources.connection(bad.body.id);
    expect(bad.status, bad.describe()).toBe(422);
    const good = await ws.connection({ properties: { 'client.id': 'acc-client' } }, ws.names.of('okprops'));
    expect((await api.get(Paths.connection(good))).body.properties).toEqual({ 'client.id': 'acc-client' });
  });

  test('TC-CONN-12 draft test runs the checklist without saving or touching health', async ({ ws, api }) => {
    const res = await api.post('/api/v1/connections/test', { name: ws.names.of('draft'), envTag: 'dev', bootstrapServers: [Profile.platformBootstrap], security: { mode: 'plaintext' } });
    expect(res.status, res.describe()).toBe(200);
    expect(TestStream.parse(res.text).result.ok).toBe(true);
    const list = await api.get(`${Paths.connections}?q=${ws.names.of('draft')}`);
    expect(list.body.items).toHaveLength(0);
  });

  test('TC-CONN-13 optimistic concurrency: stale If-Match 412, missing If-Match 428', async ({ ws, api }) => {
    const id = await ws.connection();
    const first = await api.get(Paths.connection(id));
    expect((await api.patch(Paths.connection(id), { description: 'one' }, first.etag)).status).toBe(200);
    expect((await api.patch(Paths.connection(id), { description: 'two' }, first.etag)).status).toBe(412);
    expect((await api.patch(Paths.connection(id), { description: 'three' })).status).toBe(428);
  });

  test('TC-CONN-16 schema registry credentials are write-only', async ({ ws, api }) => {
    const id = await ws.connection({ schemaRegistry: { url: 'http://127.0.0.1:18081', auth: { mode: 'basic', username: 'reg' } }, secrets: { schemaRegistrySecret: 'acc-registry-secret' } });
    const res = await api.get(Paths.connection(id));
    expect(res.body.schemaRegistry).toMatchObject({ url: 'http://127.0.0.1:18081', secret: 'set' });
    expect(res.text).not.toContain('acc-registry-secret');
  });

  test('TC-CONN-18 list filters by environment and text', async ({ ws, api }) => {
    await ws.connection({ envTag: 'staging' }, ws.names.of('stg'));
    const byText = await api.get(`${Paths.connections}?q=${ws.names.of('stg')}`);
    expect(byText.body.items.map((c: { name: string }) => c.name)).toEqual([ws.names.of('stg')]);
    const byEnv = await api.get(`${Paths.connections}?env=prod&q=${ws.names.of('stg')}`);
    expect(byEnv.body.items).toHaveLength(0);
    expect((await api.get(`${Paths.connections}?env=qa`)).status).toBeGreaterThanOrEqual(400);
  });

  test('TC-CONN-19 optional write check produces a marked test record', async ({ ws, api }) => {
    const id = await ws.connection();
    await ws.topics(id, [ws.names.inTopic]);
    const res = await api.post(Paths.connectionTest(id), { readTopic: ws.names.inTopic, writeTopic: ws.names.inTopic });
    const { steps, result } = TestStream.parse(res.text);
    expect(TestStream.finalStates(steps)).toMatchObject({ can_read: 'passed', can_write: 'passed' });
    expect(result.ok).toBe(true);
    const records = await ws.kafka.consume(ws.names.inTopic, 1, 20_000);
    expect(Object.keys(records[0]?.headers ?? {})).toContain(Conn.writeHeader);
  });
});
