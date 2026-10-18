import { describe, expect, it, vi } from 'vitest';
import { ReplayStatus } from '../../domain';
import { MockBackend } from './MockBackend';
import { MockReplayText, MockRouteReplays } from './MockRouteReplays';

describe('MockRouteReplays', () => {
  it('rejects unconfirmed, future and overlapping replays, then walks a replay to done', async () => {
    vi.useFakeTimers();
    const backend = new MockBackend();
    const replays = new MockRouteReplays(backend);
    const past = new Date(backend.clock.now().getTime() - 3_600_000).toISOString();
    const future = new Date(backend.clock.now().getTime() + 3_600_000).toISOString();
    const settle = <T,>(p: Promise<T>) => {
      vi.advanceTimersByTime(500);
      return p;
    };
    await expect(settle(replays.request('r1', { from: past, confirm: false }))).rejects.toThrow(MockReplayText.confirm);
    await expect(settle(replays.request('r1', { from: future, confirm: true }))).rejects.toThrow(MockReplayText.future);
    const replay = await settle(replays.request('r1', { from: past, confirm: true }));
    expect(replay.status).toBe(ReplayStatus.Pending);
    await expect(settle(replays.request('r1', { from: past, confirm: true }))).rejects.toThrow(MockReplayText.running);
    vi.advanceTimersByTime(MockRouteReplays.stepMs * 4);
    const done = await settle(replays.status('r1', replay.id));
    expect(done.status).toBe(ReplayStatus.Done);
    expect(done.partitions).toBe(MockRouteReplays.mockPartitions);
    vi.useRealTimers();
  });
});
