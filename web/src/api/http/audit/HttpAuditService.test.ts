import { describe, expect, it } from 'vitest';
import { AnyAuditQuery, AuditCategory, AuditExportSink, AuditFileFormat } from '../../../domain';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import { HttpAuditService } from './HttpAuditService';

const entry = {
  id: '15', at: '2026-10-14T06:15:12Z', actor: { id: 'u1', name: 'Hoa L.', kind: 'user' }, action: 'route.created',
  category: AuditCategory.Change, objectType: 'route', objectId: 'r1', objectLabel: 'orders-to-crm', teamId: 't1',
  ip: '10.0.0.1', requestId: 'req1', sensitive: false, diff: { name: 'orders-to-crm' },
};

function service(fetch: TestFetch): HttpAuditService {
  return new HttpAuditService(new HttpContext({ apiBaseUrl: '/api/v1', authBaseUrl: '', fetch: fetch.fn }));
}

describe('HttpAuditService', () => {
  it('sends filters to the server and maps names, not ids', async () => {
    const fetch = new TestFetch().json(200, { items: [entry, { ...entry, id: '14', objectType: 'session', objectId: 'dev', objectLabel: '' }], next_cursor: 'c2' });
    const page = await service(fetch).query({ ...AnyAuditQuery, text: ' orders ', actorId: 'u1', sensitiveOnly: true }, 'c1');
    const url = new URL(fetch.calls[0].url, 'http://x');
    expect(url.pathname).toBe('/api/v1/audit-log');
    expect(Object.fromEntries(url.searchParams)).toEqual({ actor: 'u1', q: 'orders', sensitive: 'true', limit: '50', cursor: 'c1' });
    expect(page.nextCursor).toBe('c2');
    expect(page.items.map((e) => [e.actor, e.object, e.teamId])).toEqual([['Hoa L.', 'orders-to-crm', 't1'], ['Hoa L.', 'session dev', 't1']]);
    expect(page.items[0].diff).toEqual({ name: 'orders-to-crm' });
  });

  it('omits empty filters and lists actors from /users', async () => {
    const fetch = new TestFetch()
      .json(200, { items: [], next_cursor: null })
      .json(200, { items: [{ id: 'u2', email: 'z@x.io', displayName: 'Zoe', teams: [] }, { id: 'u1', email: 'a@x.io', displayName: '', teams: [] }], next_cursor: null });
    const svc = service(fetch);
    await svc.query(AnyAuditQuery, null);
    expect(fetch.calls[0].url).toBe('/api/v1/audit-log?limit=50');
    expect(await svc.actors()).toEqual([{ id: 'u1', name: 'a@x.io' }, { id: 'u2', name: 'Zoe' }]);
  });

  it('builds the export URL with every filter and the format', async () => {
    const url = await service(new TestFetch()).exportUrl(
      { ...AnyAuditQuery, actions: ['sample.viewed_raw', 'livetail.opened'], objectType: 'route', teamId: 't1', from: '2026-10-13T00:00:00.000Z', sensitiveOnly: true },
      AuditFileFormat.Csv,
    );
    const parsed = new URL(url, 'http://x');
    expect(parsed.pathname).toBe('/api/v1/audit-log/export');
    expect(Object.fromEntries(parsed.searchParams)).toEqual({
      action: 'sample.viewed_raw,livetail.opened', objectType: 'route', teamId: 't1', from: '2026-10-13T00:00:00.000Z', sensitive: 'true', format: 'csv',
    });
  });

  it('reads the action catalog and the SIEM export status', async () => {
    const fetch = new TestFetch()
      .json(200, { items: [{ action: 'sample.viewed_raw', category: 'access', sensitive: true }] })
      .json(200, { enabled: true, sink: 'webhook', target: 'https://siem/in', lagRows: 3,
        checkpoint: { sink: 'webhook', lastId: '41', exportedTotal: 41, lastError: '503', lastAttemptAt: '2026-10-14T09:00:00Z', updatedAt: null } });
    const svc = service(fetch);
    expect((await svc.actions())[0].sensitive).toBe(true);
    expect(await svc.exportStatus()).toEqual({ enabled: true, sink: AuditExportSink.Webhook, target: 'https://siem/in', lagRows: 3, exportedTotal: 41,
      lastError: '503', lastAttemptAt: '2026-10-14T09:00:00Z' });
  });
});
