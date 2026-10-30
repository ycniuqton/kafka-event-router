import { describe, expect, it } from 'vitest';
import { LookupAuthMode, LookupSourceDefaults, LookupSourceInUseError, LookupSourceKind } from '../../../domain';
import { LOOKUP_SOURCES } from '../../../mocks';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import { HttpLookupSourcesService, LookupSourceInputMapper } from './HttpLookupSourcesService';

const setup = () => {
  const fetch = new TestFetch();
  const context = new HttpContext({ apiBaseUrl: '/api/v1', fetch: fetch.fn });
  return { fetch, service: new HttpLookupSourcesService(context) };
};
const body = (fetch: TestFetch, i: number) => JSON.parse(String(fetch.calls[i].init.body)) as Record<string, unknown>;

describe('HttpLookupSourcesService', () => {
  it('sends only the kind settings and typed secrets', () => {
    const d = LookupSourceDefaults.draft(LookupSourceKind.Http);
    const input = LookupSourceInputMapper.input({ ...d, name: ' crm ', http: { ...d.http, baseUrl: 'https://crm', auth: { ...d.http.auth, mode: LookupAuthMode.Bearer } }, secrets: { token: 't', password: ' ' } });
    expect(input).toMatchObject({ name: 'crm', kind: 'http', redis: null, postgres: null, static: null, secrets: { token: 't' } });
    expect(input.owner).toBeUndefined();
    expect(LookupSourceInputMapper.input({ ...d, ownerTeamId: 'team-1' }).owner).toEqual({ teamId: 'team-1' });
  });

  it('lists pages, then patches with the remembered version as If-Match', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { items: [LOOKUP_SOURCES[0]], nextCursor: null }).json(200, { ...LOOKUP_SOURCES[0], version: 4 }, { ETag: '"4"' });
    const [first] = await service.list();
    const updated = await service.update(first.id, LookupSourceDefaults.fromSource(first));
    expect(updated.version).toBe(4);
    expect(fetch.calls[1].url).toBe('/api/v1/lookup-sources/ls-crm-api');
    expect(fetch.calls[1].init.method).toBe('PATCH');
    expect(fetch.header(1, 'If-Match')).toBe('"3"');
    expect(body(fetch, 1)).not.toHaveProperty('secrets');
  });

  it('turns the in-use problem into LookupSourceInUseError', async () => {
    const { fetch, service } = setup();
    fetch.problem(409, { type: 'https://eventrouter.dev/problems/lookup-source-in-use', title: 'In use', status: 409, detail: 'Routes use it', routes: [{ id: 'r1', name: 'orders' }] });
    await expect(service.remove('ls-1')).rejects.toBeInstanceOf(LookupSourceInUseError);
  });

  it('posts saved and draft tests', async () => {
    const { fetch, service } = setup();
    const result = { ok: true, health: 'healthy', checkedAt: '', steps: [] };
    fetch.json(200, result).json(200, result);
    await service.test('ls-1');
    await service.testDraft(LookupSourceDefaults.draft());
    expect(fetch.calls.map((c) => c.url)).toEqual(['/api/v1/lookup-sources/ls-1/test', '/api/v1/lookup-sources/test']);
  });
});
