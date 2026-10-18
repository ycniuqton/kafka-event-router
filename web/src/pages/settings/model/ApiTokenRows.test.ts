import { describe, expect, it } from 'vitest';
import { ApiError } from '../../../api/http/core';
import { MockServicesFactory } from '../../../api/mock';
import { ApiTokenRules, ApiTokenScope, ApiTokenStatus, Tone, type ApiToken } from '../../../domain';
import { ApiTokenRows, ApiTokenText } from './ApiTokenRows';

const now = new Date('2026-10-14T10:00:00Z');
const base: ApiToken = {
  id: 't1', name: 'ci', prefix: 'ert_AbCd1234', scopes: [ApiTokenScope.Read], createdAt: '2026-10-01T00:00:00Z',
  expiresAt: '2026-12-30T00:00:00Z', lastUsedAt: null, status: ApiTokenStatus.Active,
};

describe('ApiTokenRows', () => {
  it('renders usage and expiry', () => {
    const [fresh, expired] = ApiTokenRows.rows([base, { ...base, id: 't2', status: ApiTokenStatus.Expired, lastUsedAt: '2026-10-14T09:58:00Z' }], now);
    expect(fresh.used).toBe(ApiTokenText.neverUsed);
    expect(fresh.expiry.startsWith(ApiTokenText.expiresPrefix)).toBe(true);
    expect(fresh.expiryTone).toBe(Tone.Off);
    expect(expired.expiry).toBe(ApiTokenText.expired);
    expect(expired.expiryTone).toBe(Tone.Err);
    expect(expired.used.startsWith(ApiTokenText.usedPrefix)).toBe(true);
  });

  it('prefers local and server name errors', () => {
    const draft = ApiTokenRules.emptyDraft();
    expect(ApiTokenRows.createError(new Error('x'), draft)).toContain('name');
    const problem = ApiError.fromProblem({ type: 'https://eventrouter.dev/problems/validation', title: 'x', status: 422, detail: '1 field',
      errors: [{ path: '/name', code: 'duplicate', message: 'Name taken' }] });
    expect(ApiTokenRows.createError(problem, { ...draft, name: 'ci' })).toBe('Name taken');
  });
});

describe('MockApiTokensService', () => {
  it('creates, lists and revokes', async () => {
    const tokens = MockServicesFactory.create().apiTokens;
    const created = await tokens.create({ name: 'ci', scope: ApiTokenScope.Write, expiresInDays: 7 });
    expect(created.secret.startsWith('ert_')).toBe(true);
    expect(created.token.prefix).toBe(created.secret.slice(0, 12));
    expect((await tokens.list()).map((t) => t.name)).toEqual(['ci']);
    await expect(tokens.create({ name: 'ci', scope: ApiTokenScope.Read, expiresInDays: 7 })).rejects.toThrow();
    await tokens.revoke(created.token.id);
    expect(await tokens.list()).toEqual([]);
  });
});
