import { describe, expect, it } from 'vitest';
import { DeadLetterErrorClass, DeadLetterJobAction, DeadLetterKind, DeadLetterReplayStatus, DryRunOutcome } from '../../../domain';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import type { DeadLetterItemDto } from './DeadLetterDtos';
import { HttpDeadLettersService } from './HttpDeadLettersService';

function item(id: string): DeadLetterItemDto {
  return {
    id,
    routeId: 'r1',
    routeName: 'Orders',
    failedAt: '2026-10-13T09:41:07Z',
    errorType: 'sandbox_crash',
    message: 'boom',
    step: 'js1',
    branch: 'main',
    versionNo: 4,
    source: { topic: 'orders', partition: 0, offset: 7 },
    resolution: null,
    errorClass: 'router',
    fingerprint: 'fp1',
    attempts: 3,
  };
}

function setup() {
  const fetch = new TestFetch();
  return { fetch, service: new HttpDeadLettersService(new HttpContext({ apiBaseUrl: '/api/v1', fetch: fetch.fn })) };
}

const job = { jobId: 'j1', routeId: 'r1', routeName: 'Orders', action: 'retry', status: 'pending_approval', total: 1, processed: 0, counts: {}, createdAt: '', finishedAt: null, requestedByMe: true, approval: { required: true } };

describe('HttpDeadLettersService workflow', () => {
  it('maps class, fingerprint and attempts and sends class / fingerprint filters', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { items: [item('a')], next_cursor: null, scanned: 1, truncated: false });
    const page = await service.page({ routeId: 'r1', kinds: [], classes: [DeadLetterErrorClass.Router], fingerprint: 'fp1' });
    expect(fetch.calls[0].url).toBe('/api/v1/routes/r1/dlq?class=router&fingerprint=fp1&limit=50');
    expect(page.items[0]).toMatchObject({ kind: DeadLetterKind.SandboxCrash, errorClass: DeadLetterErrorClass.Router, fingerprint: 'fp1', attempts: 3 });
  });

  it('reads cause groups of a route and of every route', async () => {
    const { fetch, service } = setup();
    const group = { fingerprint: 'fp1', routeId: 'r1', routeName: 'Orders', step: 'js1', stepLabel: 'Step 1 · JavaScript', errorType: 'step_error', errorClass: 'data', pattern: 'p <n>', count: 4, firstSeenAt: 'a', lastSeenAt: 'b', versions: null, sample: { id: 'a', message: 'p 1', failedAt: 'b' }, trend: { since: 's', bucketSeconds: 3600, counts: [1, 3] } };
    fetch.json(200, { items: [group], scanned: 4, truncated: false, approximate: true, byClass: { data: 4 } });
    const page = await service.groups({ routeId: 'r1', kinds: [], since: '2026-10-13T00:00:00Z', fingerprint: 'ignored' });
    expect(fetch.calls[0].url).toBe('/api/v1/routes/r1/dlq/groups?since=2026-10-13T00%3A00%3A00Z');
    expect(page.items[0]).toMatchObject({ step: 'Step 1 · JavaScript', stepId: 'js1', kind: DeadLetterKind.TransformError, errorClass: 'data', versions: [] });
    expect(page.byClass).toEqual({ data: 4 });
    fetch.json(200, { items: [], scanned: 0, truncated: false, approximate: false, byClass: null });
    await service.groups({ kinds: [], classes: [DeadLetterErrorClass.Router] });
    expect(fetch.calls[1].url).toBe('/api/v1/dlq/groups?class=router');
  });

  it('replays an edited record in its original encodings and maps dry run and approval answers', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { items: [item('a')], next_cursor: null, scanned: 1, truncated: false });
    const [d] = (await service.page({ kinds: [] })).items;
    fetch.json(200, { ...item('a'), stack: '', key: { encoding: 'utf8', data: 'k1' }, value: { encoding: 'json', data: { n: 1 } }, headers: [], masked: false });
    fetch.json(200, { dryRun: true, activeVersionNo: 5, masked: false, diff: [{ path: '/value/n', op: 'replace' }], result: { id: 'a', outcome: 'would_send', outputs: [{ branch: 'main', connectionId: 'c', topic: 't', key: null, value: { encoding: 'json', data: { n: 2 } }, headers: [] }] } });
    const preview = await service.replay(d, { key: 'k2', value: { n: 2 } }, { dryRun: true, note: '' });
    expect(fetch.calls[1].url).toBe('/api/v1/routes/r1/dlq/a');
    expect(fetch.calls[2].url).toBe('/api/v1/routes/r1/dlq/a/replay');
    expect(JSON.parse(String(fetch.calls[2].init.body))).toEqual({ edit: { key: { encoding: 'utf8', data: 'k2' }, value: { encoding: 'json', data: { n: 2 } } }, dryRun: true, note: '' });
    expect(preview).toMatchObject({ status: DeadLetterReplayStatus.Preview, activeVersionNo: 5, preview: { outcome: DryRunOutcome.WouldSend, outputs: [{ value: { n: 2 } }] } });
    fetch.json(202, { dryRun: false, status: 'pending_approval', diff: [], job });
    const pending = await service.replay(d, null, { dryRun: false, note: 'pls' });
    expect(JSON.parse(String(fetch.calls[3].init.body))).toEqual({ dryRun: false, note: 'pls' });
    expect(pending).toMatchObject({ status: DeadLetterReplayStatus.PendingApproval, job: { id: 'j1', requestedByMe: true } });
  });

  it('turns replay_approval_required into an approval request and drives approvals', async () => {
    const { fetch, service } = setup();
    fetch.json(200, { items: [item('a')], next_cursor: null, scanned: 1, truncated: false });
    await service.page({ kinds: [] });
    fetch.problem(409, { type: 'https://eventrouter.dev/problems/conflict', title: 'Conflict', status: 409, code: 'replay_approval_required' });
    const report = await service.retry(['a']);
    expect(report).toMatchObject({ resolved: [], approvalRequired: [{ routeId: 'r1', ids: ['a'] }] });
    fetch.json(202, job);
    await service.startJob('r1', DeadLetterJobAction.Retry, { kinds: [] }, { ids: ['a'], ratePerSecond: 20, note: 'n' });
    expect(JSON.parse(String(fetch.calls[2].init.body))).toEqual({ action: 'retry', ids: ['a'], ratePerSecond: 20, note: 'n' });
    fetch.json(200, { items: [job] });
    expect(await service.pendingApprovals()).toHaveLength(1);
    expect(fetch.calls[3].url).toBe('/api/v1/dlq/jobs?status=pending_approval');
    fetch.json(200, { ...job, status: 'queued' });
    await service.approveJob('j1', 'ok');
    expect(fetch.calls[4].url).toBe('/api/v1/dlq/jobs/j1/approve');
    fetch.json(200, { ...job, status: 'rejected' });
    await service.rejectJob('j1', 'no');
    expect(fetch.calls[5].url).toBe('/api/v1/dlq/jobs/j1/reject');
    fetch.json(200, { activeVersionNo: 5, sampled: 1, results: [{ id: 'a', outcome: 'failed', message: 'x', outputs: null }], counts: { failed: 1 } });
    const dry = await service.dryRun('r1', { ids: ['a'] });
    expect(fetch.calls[6].url).toBe('/api/v1/routes/r1/dlq/dry-run');
    expect(dry.results[0]).toMatchObject({ outcome: DryRunOutcome.Failed, outputs: [] });
  });
});
