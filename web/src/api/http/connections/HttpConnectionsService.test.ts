import { describe, expect, it } from 'vitest';
import { ConnectionInUseError, ConnectionStatus, EnvTag, RegistryAuthMode, ScramMechanism, SecurityMode, TestStepState, type ConnectionTestRun } from '../../../domain';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import { ConnectionProblemType, type ConnectionDto } from './ConnectionDtos';
import { ConnectionTestAssembler } from './ConnectionTestAssembler';
import { HttpConnectionsService, TestStreamInterrupted } from './HttpConnectionsService';
import { NdjsonStreamReader } from './NdjsonStreamReader';
import { RateLabel } from './ConnectionMapper';

function dto(overrides: Partial<ConnectionDto> = {}): ConnectionDto {
  return {
    id: 'c-1',
    name: 'orders-prod',
    description: '',
    envTag: 'prod',
    owner: { teamId: 't-1', teamName: 'Platform' },
    bootstrapServers: ['kafka-1:9093', 'kafka-2:9093'],
    security: { mode: 'sasl_scram_512', tls: { enabled: true }, username: 'svc', secretFields: { password: 'set' } },
    properties: {},
    sensitive: false,
    allowTopicCreate: false,
    probeEnabled: true,
    kind: 'kafka',
    health: { status: 'auth_failed', checkedAt: '2026-10-13T08:00:00Z', lastFailure: null },
    usedBy: { count: 0 },
    version: 4,
    createdAt: '',
    updatedAt: '',
    ...overrides,
  };
}

function streamOf(chunks: string[]): ReadableStream<Uint8Array> {
  const encoder = new TextEncoder();
  return new ReadableStream({
    start(controller) {
      chunks.forEach((c) => controller.enqueue(encoder.encode(c)));
      controller.close();
    },
  });
}

function setup() {
  const fetch = new TestFetch();
  const context = new HttpContext({ apiBaseUrl: '/api/v1', fetch: fetch.fn });
  return { fetch, context, service: new HttpConnectionsService(context) };
}

describe('NdjsonStreamReader', () => {
  it('reassembles lines split across chunks and keeps a trailing line', async () => {
    const lines: unknown[] = [];
    for await (const line of new NdjsonStreamReader(streamOf(['{"a":1}\n{"b"', ':2}\n\n{"c":', '3}']))) lines.push(line);
    expect(lines).toEqual([{ a: 1 }, { b: 2 }, { c: 3 }]);
  });
});

describe('ConnectionTestAssembler', () => {
  it('maps steps by id, joins message and hint, and finishes on the result line', () => {
    const a = new ConnectionTestAssembler('c-1');
    a.apply({ type: 'step', step: 'brokers_reachable', state: 'passed' });
    const run = a.apply({ type: 'step', step: 'authenticated', state: 'failed', message: 'Authentication failed.', hint: 'Check the SASL mechanism.' });
    expect(run.steps[0].state).toBe(TestStepState.Passed);
    expect(run.steps[1]).toMatchObject({ state: TestStepState.Failed, reason: 'Authentication failed. Check the SASL mechanism.' });
    expect(run.finished).toBe(false);
    const done = a.apply({ type: 'result', ok: false, health: 'auth_failed', checkedAt: '' });
    expect(done).toMatchObject({ finished: true, ok: false });
  });
});

describe('HttpConnectionsService', () => {
  it('lists every page and maps health, security and names', async () => {
    const { fetch, context, service } = setup();
    fetch.json(200, { items: [dto()], next_cursor: 'n1' }).json(200, { items: [dto({ id: 'c-2', health: { status: 'degraded', checkedAt: null, lastFailure: null } })], next_cursor: null });
    const list = await service.list();
    expect(fetch.calls[0].url).toBe('/api/v1/connections?limit=200');
    expect(list[0]).toMatchObject({ id: 'c-1', name: 'orders-prod', env: EnvTag.Prod, security: 'SASL/SCRAM-512', status: ConnectionStatus.AuthFailed, owner: 'Platform', topics: [] });
    expect(list[1].status).toBe(ConnectionStatus.Degraded);
    expect(context.etags.ifMatch('/connections/c-1')).toBe('"4"');
  });

  it('creates with write-only secrets mapped from the draft', async () => {
    const { fetch, service } = setup();
    fetch.json(201, dto({ health: { status: 'unknown', checkedAt: null, lastFailure: null } }), { ETag: '"1"' });
    const created = await service.create({
      name: 'orders-prod', description: '', env: EnvTag.Prod, team: 'Platform', bootstrapServers: ['k:9092'],
      security: SecurityMode.SaslScram, username: 'svc', password: 'pw', schemaRegistryUrl: '',
    });
    const body = JSON.parse(String(fetch.calls[0].init.body));
    expect(body).toMatchObject({ envTag: 'prod', security: { mode: 'sasl_scram_512', username: 'svc' }, secrets: { password: 'pw' } });
    expect(created.status).toBe(ConnectionStatus.Untested);
  });

  it('streams the test checklist and reports each snapshot', async () => {
    const { fetch, service } = setup();
    const lines = [
      '{"type":"step","step":"brokers_reachable","state":"running"}\n',
      '{"type":"step","step":"brokers_reachable","state":"passed","durationMs":5}\n{"type":"step","step":"authenticated","state":"passed"}\n',
      '{"type":"result","ok":true,"health":"healthy","checkedAt":"2026-10-13T08:00:00Z"}\n',
    ];
    (fetch as unknown as { queue: (() => Response)[] }).queue.push(() => new Response(streamOf(lines), { status: 200, headers: { 'Content-Type': 'application/x-ndjson' } }));
    const updates: ConnectionTestRun[] = [];
    const run = await service.test('c-1', (r) => updates.push(r));
    expect(fetch.calls[0].url).toBe('/api/v1/connections/c-1/test');
    expect(run).toMatchObject({ finished: true, ok: true });
    expect(updates[1].steps[0].state).toBe(TestStepState.Running);
    expect(updates.length).toBe(5);
  });

  it('fails when the stream ends without a result line', async () => {
    const { fetch, service } = setup();
    (fetch as unknown as { queue: (() => Response)[] }).queue.push(() => new Response(streamOf(['{"type":"step","step":"brokers_reachable","state":"running"}\n']), { status: 200 }));
    await expect(service.test('c-1', () => undefined)).rejects.toBeInstanceOf(TestStreamInterrupted);
  });

  it('lists topics with rate labels and creates topics', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { items: [{ name: 'orders', partitions: 6, replicationFactor: 3, internal: false, ratePerSec: 1840 }, { name: 'x', partitions: 1, replicationFactor: 1, internal: false, ratePerSec: null }] });
    fetch.json(201, { name: 'payments', partitions: 6, replicationFactor: 3, retentionMs: 604800000 });
    expect(await service.listTopics('c-1', { includeInternal: true })).toEqual([
      { name: 'orders', partitions: 6, rate: '1.8k/s', replicationFactor: 3, internal: false },
      { name: 'x', partitions: 1, rate: RateLabel.placeholder, replicationFactor: 1, internal: false },
    ]);
    expect(fetch.calls[0].url).toBe('/api/v1/connections/c-1/topics?includeInternal=true');
    expect(await service.createTopic('c-1', 'payments', { partitions: 6, retentionMs: 604800000 })).toEqual({
      name: 'payments',
      partitions: 6,
      rate: RateLabel.placeholder,
      replicationFactor: 3,
    });
    expect(JSON.parse(String(fetch.calls[1].init.body))).toEqual({ name: 'payments', partitions: 6, retentionMs: 604800000 });
  });

  it('maps saved security details for the edit panel', async () => {
    const { fetch, service } = setup();
    const security = { mode: 'sasl_scram_256' as const, tls: { enabled: true, caPem: 'PEM' }, username: 'svc', secretFields: { password: 'set' as const } };
    fetch.json(200, dto({ security, properties: { 'client.id': 'er' } }), { ETag: '"4"' });
    const c = await service.get('c-1');
    expect(c.securityDetail).toEqual({
      mode: SecurityMode.SaslScram,
      scram: ScramMechanism.Sha256,
      tls: { enabled: true, caPem: 'PEM', serverName: '', clientCertPem: '' },
      username: 'svc',
      storedSecrets: ['password'],
      clientId: '',
      tokenUrl: '',
      scopes: [],
    });
    expect(c.properties).toEqual({ 'client.id': 'er' });
    expect(c.schemaRegistry).toBeNull();
  });

  it('patches with If-Match, keeps blank secrets and removes dropped properties', async () => {
    const { fetch, context, service } = setup();
    context.etags.remember('/connections/c-1', '"4"');
    fetch.json(200, dto({ properties: { 'client.id': 'er', 'fetch.max.bytes': '1' } }));
    fetch.json(200, dto({ version: 5 }), { ETag: '"5"' });
    await service.update('c-1', {
      name: 'orders-prod', description: 'd', env: EnvTag.Prod, team: 'Platform', bootstrapServers: ['k:9092'],
      security: SecurityMode.SaslScram, scram: ScramMechanism.Sha256, username: 'svc', password: '', schemaRegistryUrl: '',
      tls: { enabled: true, caPem: '', serverName: '', clientCertPem: 'ignored' }, properties: { 'client.id': 'er2' },
    });
    expect(fetch.header(1, 'If-Match')).toBe('"4"');
    const body = JSON.parse(String(fetch.calls[1].init.body));
    expect(body.secrets).toBeUndefined();
    expect(body.security).toEqual({
      mode: 'sasl_scram_256',
      tls: { enabled: true, caPem: '', serverName: '', clientCertPem: '' },
      username: 'svc',
      clientId: '',
      tokenUrl: '',
      scopes: [],
    });
    expect(body.schemaRegistry).toBeNull();
    expect(body.properties).toEqual({ 'client.id': 'er2', 'fetch.max.bytes': null });
    expect(context.etags.ifMatch('/connections/c-1')).toBe('"5"');
  });

  it('turns connection-in-use into ConnectionInUseError with the routes', async () => {
    const { fetch, service } = setup();
    fetch.problem(409, { type: ConnectionProblemType.InUse, title: 'Conflict', status: 409, detail: 'in use', routes: [{ id: 'r1', name: 'orders-out' }] });
    const error = await service.remove('c-1').catch((e: unknown) => e);
    expect(error).toBeInstanceOf(ConnectionInUseError);
    expect((error as ConnectionInUseError).routes).toEqual([{ id: 'r1', name: 'orders-out' }]);
  });

  it('adds the hint of cluster-unavailable problems to the message', async () => {
    const { fetch, service } = setup();
    fetch.problem(502, { type: ConnectionProblemType.ClusterUnavailable, title: 'Bad Gateway', status: 502, detail: "Can't reach the cluster.", hint: 'Check the brokers.' });
    await expect(service.listTopics('c-1')).rejects.toThrow("Can't reach the cluster. Check the brokers.");
  });

  it('tests a draft with the edited id and the test topics', async () => {
    const { fetch, service } = setup();
    (fetch as unknown as { queue: (() => Response)[] }).queue.push(() => new Response(streamOf(['{"type":"result","ok":true,"health":"healthy","checkedAt":""}\n']), { status: 200 }));
    const draft = { name: 'n', description: '', env: EnvTag.Dev, team: 't', bootstrapServers: ['k:9092'], security: SecurityMode.None, username: '', password: '', schemaRegistryUrl: '' };
    const run = await service.testDraft(draft, 'c-1', () => undefined, { writeTopic: 'w' });
    expect(fetch.calls[0].url).toBe('/api/v1/connections/test');
    expect(JSON.parse(String(fetch.calls[0].init.body))).toMatchObject({ name: 'n', connectionId: 'c-1', writeTopic: 'w', security: { mode: 'plaintext' } });
    expect(run.ok).toBe(true);
  });

  it('sends OAuth fields, the schema registry and their write-only secrets', async () => {
    const { fetch, service } = setup();
    fetch.json(201, dto(), { ETag: '"1"' });
    await service.create({
      name: 'oauth', description: '', env: EnvTag.Dev, team: 't', bootstrapServers: ['k:9092'], security: SecurityMode.OAuth, username: 'ignored', password: 'ignored',
      clientId: 'router', tokenUrl: 'https://idp/token', scopes: ['kafka'], clientSecret: 'cs', sensitive: true,
      schemaRegistryUrl: ' https://sr:8081 ', schemaRegistryAuth: RegistryAuthMode.Bearer, schemaRegistryUsername: 'dropped', schemaRegistrySecret: 'tok',
    });
    const body = JSON.parse(String(fetch.calls[0].init.body));
    expect(body.security).toMatchObject({ mode: 'sasl_oauthbearer', username: '', clientId: 'router', tokenUrl: 'https://idp/token', scopes: ['kafka'] });
    expect(body.schemaRegistry).toEqual({ url: 'https://sr:8081', auth: { mode: 'bearer', username: '' } });
    expect(body.secrets).toEqual({ clientSecret: 'cs', schemaRegistrySecret: 'tok' });
    expect(body.sensitive).toBe(true);
  });

  it('maps the saved schema registry and OAuth identity', async () => {
    const { fetch, service } = setup();
    const security = { mode: 'sasl_oauthbearer' as const, tls: { enabled: false }, clientId: 'router', tokenUrl: 'https://idp/token', scopes: ['a', 'b'], secretFields: { clientSecret: 'set' as const } };
    fetch.json(200, dto({ security, schemaRegistry: { url: 'https://sr', auth: { mode: 'basic', username: 'u' }, secret: 'set' } }), { ETag: '"4"' });
    const c = await service.get('c-1');
    expect(c.securityDetail).toMatchObject({ mode: SecurityMode.OAuth, clientId: 'router', scopes: ['a', 'b'], storedSecrets: ['clientSecret'] });
    expect(c.schemaRegistry).toEqual({ url: 'https://sr', authMode: RegistryAuthMode.Basic, username: 'u', secretStored: true });
  });
});

describe('ConnectionTestAssembler optional steps', () => {
  it('adds the schema registry row when the server reports it', () => {
    const a = new ConnectionTestAssembler('c-1');
    expect(a.snapshot().steps).toHaveLength(5);
    a.apply({ type: 'step', step: 'schema_registry', state: 'running' });
    const run = a.apply({ type: 'step', step: 'schema_registry', state: 'failed', message: "Can't reach the schema registry.", hint: 'Check the URL.' });
    expect(run.steps).toHaveLength(6);
    expect(run.steps[5]).toMatchObject({ label: 'Schema registry reachable', state: TestStepState.Failed, message: "Can't reach the schema registry." });
    expect(a.apply({ type: 'step', step: 'unknown_step', state: 'passed' }).steps).toHaveLength(6);
  });
});
