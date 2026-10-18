import { describe, expect, it } from 'vitest';
import { ApiTokenScope, ApiTokenStatus } from '../../../domain';
import { ApiError } from '../core/ApiError';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import { HttpApiTokensService } from './HttpApiTokensService';

const dto = {
  id: 't1', name: 'ci', prefix: 'ert_AbCd1234', scopes: [ApiTokenScope.Read], created_at: '2026-10-14T09:00:00Z',
  expires_at: '2027-01-12T09:00:00Z', last_used_at: null, status: ApiTokenStatus.Active,
};

function service(fetch: TestFetch): HttpApiTokensService {
  return new HttpApiTokensService(new HttpContext({ apiBaseUrl: '/api/v1', authBaseUrl: '', fetch: fetch.fn }));
}

describe('HttpApiTokensService', () => {
  it('lists tokens mapped to the domain', async () => {
    const fetch = new TestFetch().json(200, { items: [dto], next_cursor: null, scopes: ['read', 'write', 'admin'] });
    const [token] = await service(fetch).list();
    expect(token).toEqual({ id: 't1', name: 'ci', prefix: 'ert_AbCd1234', scopes: ['read'], createdAt: dto.created_at,
      expiresAt: dto.expires_at, lastUsedAt: null, status: 'active' });
    expect(fetch.calls[0].url).toBe('/api/v1/auth/tokens');
  });

  it('creates a token and returns the one-time secret', async () => {
    const fetch = new TestFetch().json(201, { ...dto, token: 'ert_secret' });
    const created = await service(fetch).create({ name: '  ci ', scope: ApiTokenScope.Write, expiresInDays: 30 });
    expect(created.secret).toBe('ert_secret');
    expect(created.token.id).toBe('t1');
    expect(fetch.calls[0].init.method).toBe('POST');
    expect(JSON.parse(String(fetch.calls[0].init.body))).toEqual({ name: 'ci', scopes: ['write'], expires_in_days: 30 });
  });

  it('revokes by id and surfaces problems as ApiError', async () => {
    const fetch = new TestFetch().empty(204).problem(404, { type: 'https://eventrouter.dev/problems/not-found', title: 'Not Found', status: 404, detail: 'gone' });
    await service(fetch).revoke('t 1');
    expect(fetch.calls[0].url).toBe('/api/v1/auth/tokens/t%201');
    expect(fetch.calls[0].init.method).toBe('DELETE');
    await expect(service(fetch).revoke('t1')).rejects.toBeInstanceOf(ApiError);
  });
});
