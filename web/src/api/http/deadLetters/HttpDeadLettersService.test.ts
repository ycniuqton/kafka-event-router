import { describe, expect, it } from 'vitest';
import { DeadLetterFileFormat, DeadLetterJobAction, DeadLetterJobs, DeadLetterKind, DeadLetterResolutionState } from '../../../domain';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import type { DeadLetterItemDto } from './DeadLetterDtos';
import { HttpDeadLettersService } from './HttpDeadLettersService';

function item(id: string, routeId: string): DeadLetterItemDto {
  return {
    id,
    routeId,
    routeName: 'Order totals',
    failedAt: '2026-10-13T09:41:07Z',
    errorType: 'step_error',
    message: 'boom',
    step: 'Step 1 · JavaScript',
    branch: 'main',
    versionNo: 4,
    source: { topic: 'order-events', partition: 2, offset: 884162 },
    resolution: null,
  };
}

function setup() {
  const fetch = new TestFetch();
  return { fetch, service: new HttpDeadLettersService(new HttpContext({ apiBaseUrl: '/api/v1', fetch: fetch.fn })) };
}

describe('HttpDeadLettersService', () => {
  it('reads one filtered page of the global queue and maps rows with step labels', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { items: [{ ...item('a', 'r1'), step: 'step-0', stepLabel: 'Step 1 · JavaScript' }], next_cursor: 'c2', scanned: 1, truncated: true });
    const page = await service.page({ kinds: [DeadLetterKind.TransformError, DeadLetterKind.Timeout], since: '2026-10-13T00:00:00Z' });
    expect(fetch.calls[0].url).toBe('/api/v1/dlq?errorType=step_error%2Ctimeout&since=2026-10-13T00%3A00%3A00Z&limit=50');
    expect(page).toMatchObject({ nextCursor: 'c2', truncated: true });
    expect(page.items[0]).toMatchObject({ id: 'a', kind: DeadLetterKind.TransformError, step: 'Step 1 · JavaScript', partition: 2, offset: 884162, routeName: 'Order totals' });
    fetch.json(200, { items: [], next_cursor: null, scanned: 0, truncated: false });
    await service.page({ routeId: 'r1', kinds: [] }, 'c2');
    expect(fetch.calls[1].url).toBe('/api/v1/routes/r1/dlq?limit=50&cursor=c2');
  });

  it('maps global and route summaries to kind counts', async () => {
    const { fetch, service } = setup();
    fetch.json(200, {
      unresolved: 3,
      byErrorType: { step_error: 2, timeout: 1 },
      approximate: true,
      routes: [{ routeId: 'r1', routeName: 'Orders', unresolved: 3, byErrorType: { step_error: 2, timeout: 1 }, approximate: true }],
    });
    const global = await service.summary();
    expect(fetch.calls[0].url).toBe('/api/v1/dlq/summary');
    expect(global).toMatchObject({ unresolved: 3, approximate: true, byKind: [{ kind: DeadLetterKind.TransformError, count: 2 }, { kind: DeadLetterKind.Timeout, count: 1 }] });
    expect(global.routes[0]).toMatchObject({ routeId: 'r1', routeName: 'Orders', unresolved: 3 });
    fetch.json(200, { unresolved: 0, byErrorType: {}, approximate: false });
    expect(await service.summary('r1')).toMatchObject({ unresolved: 0, routes: [] });
    expect(fetch.calls[1].url).toBe('/api/v1/routes/r1/dlq/summary');
  });

  it('retries per route and keeps failed events out of the resolved set', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { items: [item('a', 'r1'), item('b', 'r1'), item('c', 'r1')], next_cursor: null, scanned: 3, truncated: false });
    await service.page({ kinds: [] });
    fetch.json(200, {
      activeVersionNo: 5,
      results: [
        { id: 'a', outcome: 'sent' },
        { id: 'b', outcome: 'failed', errorType: 'step_error', message: 'still broken' },
        { id: 'c', outcome: 'skipped', reason: 'already_resolved' },
      ],
      counts: { sent: 1, failed: 1, skipped: 1, filtered: 0 },
      remaining: 0,
    });
    const report = await service.retry(['a', 'b', 'c']);
    expect(fetch.calls[1].url).toBe('/api/v1/routes/r1/dlq/retry');
    expect(JSON.parse(String(fetch.calls[1].init.body))).toEqual({ ids: ['a', 'b', 'c'] });
    expect(report).toMatchObject({ resolved: ['a'], sent: 1, failed: [{ id: 'b', message: 'still broken' }], skipped: [{ id: 'c', reason: 'already_resolved' }] });
  });

  it('retries all matching events of a route with a match selection', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { activeVersionNo: 5, results: [{ id: 'a', outcome: 'sent' }], counts: { sent: 1 }, remaining: 7 });
    const report = await service.retryMatching('r1', { kinds: [DeadLetterKind.Timeout], since: '2026-10-13T00:00:00Z' });
    expect(JSON.parse(String(fetch.calls[0].init.body))).toEqual({ match: { errorType: ['timeout'], since: '2026-10-13T00:00:00Z' } });
    expect(report).toMatchObject({ resolved: ['a'], remaining: 7 });
  });

  it('loads detail, discards, exports and opens the studio link', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { items: [item('a', 'r1')], next_cursor: null, scanned: 1, truncated: false });
    const [d] = (await service.page({ routeId: 'r1', kinds: [] })).items;
    fetch.json(200, { ...item('a', 'r1'), stack: 'at x', key: { encoding: 'utf8', data: 'k' }, value: { encoding: 'base64', data: '3q2+7w==' }, headers: [], masked: false });
    const detail = await service.detail(d);
    expect(detail).toMatchObject({ stack: 'at x', original: { key: 'k', rawBytes: true } });
    fetch.json(200, { discarded: 1, skipped: [], remaining: 0 });
    expect((await service.discard(['a'])).resolved).toEqual(['a']);
    fetch.json(200, { sessionId: 'ses_1', sampleIndex: 0, builderUrl: '/routes/r1/edit?step=transform&session=ses_1&sample=0' });
    expect(await service.openInStudio(d)).toMatchObject({ sessionId: 'ses_1', builderUrl: '/routes/r1/edit?step=transform&session=ses_1&sample=0' });
    expect(fetch.calls[3].url).toBe('/api/v1/routes/r1/dlq/a/studio');
    fetch.json(200, []);
    await service.exportJson('r1', { filter: { routeId: 'r1', kinds: [DeadLetterKind.Timeout] } });
    expect(fetch.calls[4].url).toBe('/api/v1/routes/r1/dlq/export?errorType=timeout&format=json');
  });

  it('sends P2 filters, restores, maps resolution / masked detail and drives jobs', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { items: [{ ...item('a', 'r1'), resolution: { state: 'discarded', by: 'u1', at: '2026-10-13T10:00:00Z' } }], next_cursor: null, scanned: 1, truncated: false });
    const page = await service.page({ routeId: 'r1', kinds: [], stepId: 'step-0', versionNo: 2, text: 'boom', includeResolved: true });
    expect(fetch.calls[0].url).toBe('/api/v1/routes/r1/dlq?step=step-0&version=2&q=boom&includeResolved=true&limit=50');
    expect(page.items[0].resolution).toMatchObject({ state: DeadLetterResolutionState.Discarded, by: 'u1' });
    fetch.json(200, { restored: 1, skipped: [], remaining: 0 });
    expect((await service.restore(['a'])).resolved).toEqual(['a']);
    expect(fetch.calls[1].url).toBe('/api/v1/routes/r1/dlq/restore');
    const detail = { ...item('a', 'r1'), stack: '', key: { encoding: 'utf8', data: '**' }, value: { encoding: 'avro', data: { n: 0 }, schemaId: 7 }, headers: [], masked: true };
    fetch.json(200, detail);
    expect((await service.detail(page.items[0])).original).toMatchObject({ masked: true, schema: { format: 'avro', schemaId: 7 } });
    const job = { jobId: 'j1', routeId: 'r1', routeName: 'Orders', action: 'retry', status: 'queued', total: 0, processed: 0, counts: {}, createdAt: '', finishedAt: null };
    fetch.json(202, job);
    expect(await service.startJob('r1', DeadLetterJobAction.Retry, { kinds: [], text: 'boom' })).toMatchObject({ id: 'j1', status: 'queued' });
    expect(fetch.calls[3].url).toBe('/api/v1/routes/r1/dlq/jobs');
    expect(JSON.parse(fetch.calls[3].init?.body as string)).toEqual({ action: 'retry', match: { errorType: [], q: 'boom' } });
    fetch.json(200, { ...job, status: 'succeeded', total: 5, processed: 5 });
    expect(DeadLetterJobs.percent(await service.job('j1'))).toBe(100);
    expect(fetch.calls[4].url).toBe('/api/v1/dlq/jobs/j1');
    fetch.json(200, { ...job, status: 'cancelled' });
    await service.cancelJob('j1');
    expect(fetch.calls[5].url).toBe('/api/v1/dlq/jobs/j1/cancel');
    fetch.json(200, []);
    await service.exportJson('r1', { ids: ['a'] }, DeadLetterFileFormat.Ndjson);
    expect(fetch.calls[6].url).toBe('/api/v1/routes/r1/dlq/export?ids=a&format=ndjson');
  });
});
