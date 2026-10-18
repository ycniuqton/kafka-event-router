import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../api/http/core';
import { ClusterKind, ConnectionDisplay, ConnectionStatus, EnvTag, RegistryAuthMode, ScramMechanism, SecretField, SecurityMode, type Connection } from '../../../domain';
import { TopicBrowserModel } from '../topics/TopicBrowserModel';
import { BootstrapListParser } from './BootstrapListParser';
import { ConnectionFieldErrors } from './ConnectionFieldErrors';
import { ConnectionFilter, FilterAll } from './ConnectionFilter';
import { ClientProperties, ConnectionFormModel, ConnectionFormStep, OAuthScopes } from './ConnectionForm';
import { PeekMasking } from './ConnectionDrawer';
import { ConnectionGrants, UnavailableGrantsSource } from './ConnectionGrants';

function connection(overrides: Partial<Connection> = {}): Connection {
  return {
    id: 'c-1', name: 'orders-prod', env: EnvTag.Prod, bootstrapServers: ['k1:9092'], security: 'SASL/SCRAM-512',
    status: ConnectionStatus.Healthy, checkedAt: null, owner: 'Platform', topics: [], ...overrides,
  };
}

function problem(status: number, errors: { path: string; message: string }[]): ApiError {
  return ApiError.fromProblem({ type: 'https://eventrouter.dev/problems/validation', title: 'Invalid', status, detail: 'Invalid input', errors } as never);
}

describe('BootstrapListParser', () => {
  it('splits pasted lists, dedupes and flags non host:port entries', () => {
    const servers = BootstrapListParser.add(['a:9092'], 'a:9092, b:9092\nc  [::1]:9093 d:99999');
    expect(servers).toEqual(['a:9092', 'b:9092', 'c', '[::1]:9093', 'd:99999']);
    expect(BootstrapListParser.invalid(servers)).toEqual(['c', 'd:99999']);
  });
});

describe('ConnectionFieldErrors', () => {
  it('maps problem paths to fields and the first failing step', () => {
    const view = ConnectionFieldErrors.from(problem(422, [{ path: '/security/username', message: 'Required' }, { path: '/properties/foo', message: 'Unknown' }]));
    expect(view.step).toBe(ConnectionFormStep.Security);
    expect(view.fields).toEqual({ username: 'Required', 'properties.foo': 'Unknown' });
    expect(ConnectionFieldErrors.ofProperty(view.fields, 'foo')).toBe('Unknown');
    expect(ConnectionFieldErrors.of(view.fields, 'properties')).toBe('Unknown');
  });

  it('points bootstrap server errors at the chip and the brokers step', () => {
    const view = ConnectionFieldErrors.from(problem(422, [{ path: '/bootstrapServers/1', message: 'host:port expected' }]));
    expect(view).toMatchObject({ step: ConnectionFormStep.Brokers, fields: { 'bootstrapServers.1': 'host:port expected' } });
  });

  it('keeps plain errors as a message only', () => {
    expect(ConnectionFieldErrors.from(new Error('boom'))).toEqual({ fields: {}, step: null, message: 'boom', stale: false });
  });
});

describe('ConnectionFormModel', () => {
  it('round-trips a saved connection into a draft without leaking stored secrets', () => {
    const form = ConnectionFormModel.fromConnection(
      connection({
        properties: { 'client.id': 'er' },
        allowTopicCreate: true,
        securityDetail: {
          mode: SecurityMode.SaslScram, scram: ScramMechanism.Sha256, username: 'svc', storedSecrets: ['password'],
          tls: { enabled: true, caPem: 'PEM', serverName: '', clientCertPem: '' },
        },
      }),
    );
    expect(form.password.stored).toBe(true);
    const draft = ConnectionFormModel.toDraft(form);
    expect(draft).toMatchObject({ security: SecurityMode.SaslScram, scram: ScramMechanism.Sha256, password: '', allowTopicCreate: true, properties: { 'client.id': 'er' } });
    expect(draft.tls).toEqual({ enabled: true, caPem: 'PEM', serverName: '', clientCertPem: '' });
  });

  it('drops blank property rows and blocks invalid brokers', () => {
    expect(ClientProperties.toMap([{ id: 1, key: ' a ', value: '1' }, { id: 2, key: '', value: 'x' }])).toEqual({ a: '1' });
    const form = { ...ConnectionFormModel.empty(), name: 'x', servers: ['nope'] };
    expect(ConnectionFormModel.blocker(form, ConnectionFormStep.Brokers, [])).not.toBeNull();
  });
});

describe('ConnectionFilter', () => {
  it('filters by text, env and status and counts per status', () => {
    const list = [connection(), connection({ id: 'c-2', name: 'dev-x', env: EnvTag.Dev, status: ConnectionStatus.Unreachable })];
    expect(ConnectionFilter.apply(list, { q: 'dev', env: FilterAll, status: FilterAll }).map((c) => c.id)).toEqual(['c-2']);
    expect(ConnectionFilter.apply(list, { q: '', env: EnvTag.Prod, status: FilterAll }).map((c) => c.id)).toEqual(['c-1']);
    expect(ConnectionFilter.statusCount(list, ConnectionFilter.initial, ConnectionStatus.Unreachable)).toBe(1);
  });
});

describe('TopicBrowserModel', () => {
  it('validates topic names and builds settings with 7 d retention', () => {
    expect(TopicBrowserModel.validate({ ...TopicBrowserModel.emptyForm('bad name'), partitions: '0' })).toMatchObject({ name: expect.any(String), partitions: expect.any(String) });
    expect(TopicBrowserModel.settings(TopicBrowserModel.emptyForm('ok'))).toEqual({ partitions: 6, retentionMs: 604800000 });
    expect(TopicBrowserModel.canCreate(connection({ allowTopicCreate: false }))).toBe(false);
  });
});

describe('P2 connection settings (OAuth, schema registry, sensitive, grants, masking)', () => {
  it('round-trips OAuth and schema registry settings through the form', () => {
    const saved = connection({
      sensitive: true,
      securityDetail: {
        mode: SecurityMode.OAuth, scram: ScramMechanism.Sha512, tls: { enabled: false, caPem: '', serverName: '', clientCertPem: '' }, username: '',
        storedSecrets: [SecretField.ClientSecret], clientId: 'router', tokenUrl: 'https://idp/token', scopes: ['kafka', 'read'],
      },
      schemaRegistry: { url: 'https://sr', authMode: RegistryAuthMode.Basic, username: 'u', secretStored: true },
    });
    const form = ConnectionFormModel.fromConnection(saved);
    expect(form).toMatchObject({ sensitive: true, clientId: 'router', scopes: 'kafka read', registryAuth: RegistryAuthMode.Basic, registryUser: 'u' });
    expect(form.clientSecret.stored).toBe(true);
    expect(form.registryPassword.stored).toBe(true);
    const draft = ConnectionFormModel.toDraft({ ...form, scopes: 'kafka, write' });
    expect(draft).toMatchObject({ security: SecurityMode.OAuth, scopes: ['kafka', 'write'], clientSecret: '', schemaRegistryUrl: 'https://sr', schemaRegistrySecret: '', sensitive: true });
  });

  it('maps OAuth and registry problem paths to their fields and steps', () => {
    const oauth = ConnectionFieldErrors.from(problem(422, [{ path: '/security/tokenUrl', message: 'url' }, { path: '/secrets/clientSecret', message: 'secret' }]));
    expect(oauth.fields).toEqual({ tokenUrl: 'url', clientSecret: 'secret' });
    expect(oauth.step).toBe(ConnectionFormStep.Security);
    const registry = ConnectionFieldErrors.from(problem(422, [{ path: '/secrets/schemaRegistrySecret', message: 'token' }]));
    expect(registry.fields).toEqual({ registrySecret: 'token' });
    expect(registry.step).toBe(ConnectionFormStep.SchemaRegistry);
  });

  it('splits scopes on spaces and commas', () => {
    expect(OAuthScopes.split(' a, b  c,')).toEqual(['a', 'b', 'c']);
    expect(OAuthScopes.join(['a', 'b'])).toBe('a b');
  });

  it('masks peeks on sensitive connections and for roles without raw access', () => {
    expect(PeekMasking.masked(false, true)).toBe(false);
    expect(PeekMasking.masked(true, true)).toBe(true);
    expect(PeekMasking.masked(false, false)).toBe(true);
    expect(PeekMasking.reason(true)).toContain('sensitive');
  });

  it('reports grants as unknown until access-control plugs in its source', async () => {
    expect(await ConnectionGrants.current().grantedTeams('c-1')).toBeNull();
    const source = { grantedTeams: async () => [{ teamId: 't-2', teamName: 'Analytics' }] };
    ConnectionGrants.use(source);
    expect(await ConnectionGrants.current().grantedTeams('c-1')).toEqual([{ teamId: 't-2', teamName: 'Analytics' }]);
    ConnectionGrants.use(new UnavailableGrantsSource());
  });
});

describe('Demo connections (P3)', () => {
  it('flags demo connections and makes their row read-only except test, topics and delete', () => {
    const demo = connection({ kind: ClusterKind.Demo, name: 'demo-orders' });
    expect(ConnectionDisplay.isDemo(demo)).toBe(true);
    expect(ConnectionDisplay.isDemo(connection({ kind: ClusterKind.Kafka }))).toBe(false);
    expect(ConnectionDisplay.isDemo(connection())).toBe(false);
    expect(ConnectionDisplay.editable(demo)).toBe(false);
    expect(ConnectionDisplay.editable(connection())).toBe(true);
  });
});
