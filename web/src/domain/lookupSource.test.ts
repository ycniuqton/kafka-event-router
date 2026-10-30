import { describe, expect, it } from 'vitest';
import {
  LookupAuthMode,
  LookupKindSettingsOf,
  LookupSecretField,
  LookupSourceDefaults,
  LookupSourceKind,
  LookupSourceKindMeta,
  LookupSourceMessage,
  LookupSourceRules,
  type LookupSourceDraft,
} from './lookupSource';

const http = (patch: Partial<LookupSourceDraft['http']> = {}): LookupSourceDraft => {
  const d = LookupSourceDefaults.draft(LookupSourceKind.Http);
  return { ...d, name: 'crm-api', http: { ...d.http, baseUrl: 'https://crm.internal', ...patch } };
};

describe('LookupSourceRules', () => {
  it('accepts a complete HTTP draft and flags name, URL and limits by JSON Pointer', () => {
    expect(LookupSourceRules.validate(http())).toEqual({});
    const bad = { ...http({ baseUrl: 'crm' }), name: ' ', limits: { ...LookupSourceDefaults.limits, maxConcurrency: 0, ratePerSec: -1 } };
    expect(LookupSourceRules.validate(bad)).toEqual({
      '/name': LookupSourceMessage.Name,
      '/http/baseUrl': LookupSourceMessage.BaseUrl,
      '/limits/maxConcurrency': LookupSourceMessage.Positive,
      '/limits/ratePerSec': LookupSourceMessage.NonNegative,
    });
  });

  it('needs the secret of the auth mode unless it is already stored', () => {
    const bearer = http({ auth: { mode: LookupAuthMode.Bearer, username: '', headerName: '' } });
    expect(LookupSourceRules.secretFields(bearer)).toEqual([LookupSecretField.Token]);
    expect(LookupSourceRules.validate(bearer)).toEqual({ '/secrets/token': LookupSourceMessage.Secret });
    expect(LookupSourceRules.validate(bearer, [LookupSecretField.Token])).toEqual({});
    expect(LookupSourceRules.validate({ ...bearer, secrets: { token: 't' } })).toEqual({});
    const header = http({ auth: { mode: LookupAuthMode.Header, username: '', headerName: '' } });
    expect(LookupSourceRules.validate(header)).toMatchObject({ '/http/auth/headerName': LookupSourceMessage.HeaderName, '/secrets/headerValue': LookupSourceMessage.Secret });
  });

  it('validates redis, postgres and static settings', () => {
    const base = { ...LookupSourceDefaults.draft(), name: 'x' };
    expect(LookupSourceRules.validate({ ...base, kind: LookupSourceKind.Redis, redis: { ...base.redis, addr: 'redis', db: 16 } })).toEqual({
      '/redis/addr': LookupSourceMessage.Addr, '/redis/db': LookupSourceMessage.Db,
    });
    const pg = { ...base, kind: LookupSourceKind.Postgres, postgres: { ...base.postgres, host: 'db', database: 'ref', user: 'ro' }, secrets: { password: 'p' } };
    expect(LookupSourceRules.validate(pg)).toEqual({});
    expect(LookupSourceRules.validate({ ...pg, postgres: { ...pg.postgres, port: Number.NaN } })).toEqual({ '/postgres/port': LookupSourceMessage.Port });
    expect(LookupSourceRules.validate({ ...base, kind: LookupSourceKind.Static })).toEqual({ '/static/tableRef': LookupSourceMessage.Table });
  });
});

describe('LookupKindSettingsOf + defaults', () => {
  it('sends only the settings of the draft kind', () => {
    const d = { ...LookupSourceDefaults.draft(LookupSourceKind.Static), static: { tableRef: 'price-list', version: 3 } };
    expect(LookupKindSettingsOf.draft(d)).toEqual({ http: null, redis: null, postgres: null, static: { tableRef: 'price-list', version: 3 } });
    expect(LookupSourceKindMeta[LookupSourceKind.Static].target(LookupKindSettingsOf.draft(d))).toBe('price-list v3');
  });
});
