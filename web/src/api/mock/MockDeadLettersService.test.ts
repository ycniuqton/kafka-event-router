import { describe, expect, it } from 'vitest';
import { DeadLetterJobAction, DeadLetterJobStatus, DeadLetterJobs, DeadLetterKind, DeadLetterResolutionState, DeadLetterSkipReason } from '../../domain';
import { MockBackend } from './MockBackend';
import { MockDeadLettersService, MockValueMasker } from './MockDeadLettersService';

describe('MockDeadLettersService', () => {
  it('filters by route and kind, pages and summarises', async () => {
    const service = new MockDeadLettersService(new MockBackend());
    const all = await service.page({ kinds: [] });
    expect(all.items.length).toBeGreaterThan(0);
    expect(all.items[0].routeName).toBeTruthy();
    const timeouts = await service.page({ kinds: [DeadLetterKind.Timeout] });
    expect(timeouts.items.every((d) => d.kind === DeadLetterKind.Timeout)).toBe(true);
    const summary = await service.summary();
    expect(summary.unresolved).toBe(all.items.length);
    expect(summary.routes.reduce((n, r) => n + r.unresolved, 0)).toBe(summary.unresolved);
  });

  it('retries all matching events of a route and removes them', async () => {
    const service = new MockDeadLettersService(new MockBackend());
    const [first] = (await service.page({ kinds: [] })).items;
    const report = await service.retryMatching(first.routeId, { kinds: [] });
    expect(report.resolved.length).toBeGreaterThan(0);
    expect((await service.page({ routeId: first.routeId, kinds: [] })).items).toEqual([]);
  });

  it('keeps resolved events for "show resolved", restores discarded ones and filters by text', async () => {
    const service = new MockDeadLettersService(new MockBackend());
    const [first, second] = (await service.page({ kinds: [] })).items;
    await service.discard([first.id]);
    await service.retry([second.id]);
    const resolved = await service.page({ kinds: [], includeResolved: true });
    expect(resolved.items.find((d) => d.id === first.id)?.resolution?.state).toBe(DeadLetterResolutionState.Discarded);
    const restore = await service.restore([first.id, second.id]);
    expect(restore.resolved).toEqual([first.id]);
    expect(restore.skipped).toEqual([{ id: second.id, reason: DeadLetterSkipReason.NotDiscarded }]);
    expect((await service.page({ kinds: [] })).items.some((d) => d.id === first.id)).toBe(true);
    expect((await service.page({ kinds: [], text: 'NO SUCH TEXT' })).items).toEqual([]);
  });

  it('runs a background job per poll and masks sensitive values type-preserving', async () => {
    const service = new MockDeadLettersService(new MockBackend());
    const [first] = (await service.page({ kinds: [] })).items;
    let job = await service.startJob(first.routeId, DeadLetterJobAction.Discard, { kinds: [] });
    while (!DeadLetterJobs.done(job)) job = await service.job(job.id);
    expect(job).toMatchObject({ status: DeadLetterJobStatus.Succeeded, processed: job.total });
    expect((await service.page({ routeId: first.routeId, kinds: [] })).items).toEqual([]);
    expect(MockValueMasker.mask({ s: 'abc', n: 5, b: true, a: [1] })).toEqual({ s: '•••', n: 0, b: false, a: [0] });
  });
});
