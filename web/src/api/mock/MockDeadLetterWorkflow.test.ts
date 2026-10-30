import { describe, expect, it } from 'vitest';
import { DeadLetterErrorClass, DeadLetterJobAction, DeadLetterJobStatus, DeadLetterJobs, DeadLetterReplayStatus, DryRunOutcome } from '../../domain';
import { MockBackend } from './MockBackend';
import { MockDeadLettersService, MockPendingReplay } from './MockDeadLettersService';

describe('MockDeadLettersService workflow', () => {
  it('groups causes, classifies rows and filters by class and fingerprint', async () => {
    const service = new MockDeadLettersService(new MockBackend());
    const groups = await service.groups({ kinds: [] });
    const top = groups.items[0];
    expect(top.count).toBeGreaterThanOrEqual(groups.items[groups.items.length - 1].count);
    expect(groups.byClass[DeadLetterErrorClass.Router]).toBeGreaterThan(0);
    const events = await service.page({ kinds: [], fingerprint: top.fingerprint });
    expect(events.items).toHaveLength(top.count);
    const routerOnly = await service.page({ kinds: [], classes: [DeadLetterErrorClass.Router] });
    expect(routerOnly.items.every((d) => d.errorClass === DeadLetterErrorClass.Router)).toBe(true);
    expect((await service.summary()).byClass?.[DeadLetterErrorClass.Transient]).toBeGreaterThan(0);
  });

  it('dry-runs an edited event without resolving it, then replays it', async () => {
    const service = new MockDeadLettersService(new MockBackend());
    const [d1] = (await service.page({ routeId: 'r3', kinds: [] })).items;
    const unchanged = await service.replay(d1, null, { dryRun: true, note: '' });
    expect(unchanged.preview?.outcome).toBe(DryRunOutcome.Failed);
    const edited = await service.replay(d1, { value: { items: [] } }, { dryRun: true, note: '' });
    expect(edited).toMatchObject({ status: DeadLetterReplayStatus.Preview, preview: { outcome: DryRunOutcome.WouldSend } });
    expect(edited.diff.length).toBeGreaterThan(0);
    expect((await service.page({ routeId: 'r3', kinds: [] })).items.some((d) => d.id === d1.id)).toBe(true);
    const done = await service.replay(d1, { value: { items: [] } }, { dryRun: false, note: 'fixed' });
    expect(done.status).toBe(DeadLetterReplayStatus.Done);
    expect((await service.page({ routeId: 'r3', kinds: [] })).items.some((d) => d.id === d1.id)).toBe(false);
    const sample = await service.dryRun('r3', { filter: { kinds: [] } });
    expect(sample.sampled).toBeGreaterThan(0);
  });

  it('enforces strict 4-eyes on routes writing to prod', async () => {
    const service = new MockDeadLettersService(new MockBackend());
    const report = await service.retry(['d8']);
    expect(report.approvalRequired).toEqual([{ routeId: 'r5', ids: ['d8'] }]);
    expect(report.resolved).toEqual([]);
    const replay = await service.replay((await service.page({ routeId: 'r5', kinds: [] })).items[0], null, { dryRun: false, note: '' });
    expect(replay.status).toBe(DeadLetterReplayStatus.PendingApproval);
    const own = replay.job!;
    await expect(service.approveJob(own.id, '')).rejects.toThrow(/own replay/);
    const pending = await service.pendingApprovals();
    expect(pending.map((j) => j.id)).toContain(MockPendingReplay.id);
    let job = await service.approveJob(MockPendingReplay.id, 'ok');
    expect(job.status).toBe(DeadLetterJobStatus.Queued);
    while (!DeadLetterJobs.done(job)) job = await service.job(job.id);
    expect(job.status).toBe(DeadLetterJobStatus.Succeeded);
    const rejected = await service.rejectJob(own.id, 'no').catch((e: Error) => e);
    expect(rejected).toBeInstanceOf(Error);
  });

  it('starts rate-limited jobs over ids', async () => {
    const service = new MockDeadLettersService(new MockBackend());
    const job = await service.startJob('r3', DeadLetterJobAction.Retry, { kinds: [] }, { ids: ['d1', 'd2'], ratePerSecond: 10, note: 'n' });
    expect(job).toMatchObject({ total: 2, ratePerSecond: 10, idsCount: 2, status: DeadLetterJobStatus.Queued });
  });
});
