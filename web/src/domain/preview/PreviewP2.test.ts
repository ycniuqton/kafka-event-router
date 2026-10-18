import { describe, expect, it } from 'vitest';
import { BuilderStep } from '../builder';
import { BranchResults } from './BranchResults';
import { MaskedValues } from './MaskedValues';
import { SampleSummary } from './SampleSummary';
import { ProjectedState, StepProjection } from './StepProjection';
import { EventStatus, MaskToken, RawEncoding, type EventResult, type RunResult, type SampleRecord } from './types';

const record: SampleRecord = {
  seq: 0, topic: 'orders', partition: 0, offset: 1, timestamp: '2026-10-14T00:00:00Z', key: null,
  value: { amount: 1500, type: 'sale' }, headers: [], raw: { key: null, value: '{}', encoding: RawEncoding.Utf8 }, decodeError: null,
};

const twoBranches: EventResult = {
  seq: 0,
  status: EventStatus.Passed,
  error: null,
  branches: [
    { branch: 'high-value', status: EventStatus.Passed, filter: [], steps: [{ index: 0, id: 'tag', kind: 'set', patch: [{ op: 'add', path: '/value/tier', value: 'gold' }], durationUs: 1, logs: [], error: null }], error: null, outputs: [] },
    { branch: 'refunds', status: EventStatus.Filtered, filter: [{ path: '/branches/1/filter', result: false }], steps: [], error: null, outputs: [] },
  ],
};

const run: RunResult = {
  summary: { loaded: 1, processed: 1, passed: 1, filtered: 0, errors: 0, dropped: 0, incomplete: false, durationMs: 1,
    branches: [{ branch: 'high-value', passed: 1, filtered: 0, errors: 0, dropped: 0 }, { branch: 'refunds', passed: 0, filtered: 1, errors: 0, dropped: 2 }] },
  issues: [],
  results: [twoBranches],
};

describe('per-branch results (FR-14)', () => {
  it('lists branches and their counts', () => {
    expect(BranchResults.ids(run)).toEqual(['high-value', 'refunds']);
    expect(SampleSummary.branchLabel(BranchResults.summary(run, 'refunds')!)).toBe('0 pass · 1 filtered · 0 errors · 2 dropped');
    expect(BranchResults.ids(null)).toEqual([]);
  });

  it('projects an event through the selected branch', () => {
    const first = StepProjection.project(record, twoBranches, { step: BuilderStep.Destination });
    expect(first.state).toBe(ProjectedState.Passed);
    expect((first.document.value as Record<string, unknown>).tier).toBe('gold');
    const refunds = StepProjection.project(record, twoBranches, { step: BuilderStep.Destination, branch: 'refunds' });
    expect(refunds.state).toBe(ProjectedState.Filtered);
    const unknown = StepProjection.project(record, twoBranches, { step: BuilderStep.Filter, branch: 'gone' });
    expect(unknown.state).toBe(ProjectedState.Passed);
  });
});

describe('masked values (FR-24)', () => {
  it('recognises the server mask tokens', () => {
    expect(MaskedValues.isMasked(MaskToken.String)).toBe(true);
    expect(MaskedValues.isMasked('alice')).toBe(false);
    expect(MaskedValues.containsMasked({ a: [1, { b: MaskToken.Redacted }] })).toBe(true);
    expect(MaskedValues.containsMasked({ a: 'x' })).toBe(false);
  });
});
