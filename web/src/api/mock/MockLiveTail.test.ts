import { describe, expect, it } from 'vitest';
import { TailOutcome } from '../../domain';
import { MockBackend } from './MockBackend';
import { MockOnboardingService, MockTailRecords } from './MockLiveTail';

describe('MockLiveTail', () => {
  it('cycles deterministic outcomes with outputs only for sent records', () => {
    const outcomes = Array.from({ length: 13 }, (_, i) => MockTailRecords.outcome(i));
    expect(outcomes).toContain(TailOutcome.Filtered);
    expect(outcomes).toContain(TailOutcome.DeadLettered);
    expect(outcomes.at(-1)).toBe(TailOutcome.Error);
    expect(MockTailRecords.record(0, '2026-10-14T10:00:00Z', 'out').outputs).toHaveLength(1);
    expect(MockTailRecords.record(3, '2026-10-14T10:00:00Z', 'out').outputs).toHaveLength(0);
  });

  it('creates the demo connection once and removes it', async () => {
    const svc = new MockOnboardingService(new MockBackend());
    expect((await svc.get()).demo).toBeNull();
    const seed = await svc.createDemo();
    expect(seed.topic).toBe('order-events');
    await expect(svc.createDemo()).rejects.toThrow();
    await svc.removeDemo();
    expect((await svc.get()).demo).toBeNull();
  });
});
