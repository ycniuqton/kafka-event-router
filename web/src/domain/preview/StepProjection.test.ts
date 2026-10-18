import { describe, expect, it } from 'vitest';
import { BuilderStep } from '../builder';
import { FilterTrace } from './FilterTrace';
import { SampleSummary } from './SampleSummary';
import { ProjectedState, StepProjection } from './StepProjection';
import { EventStatus, RawEncoding, StopReason, type EventResult, type RunSummary, type SampleRecord } from './types';

const record: SampleRecord = {
  seq: 0,
  topic: 'orders',
  partition: 1,
  offset: 10,
  timestamp: '2026-10-14T00:00:00Z',
  key: 'k1',
  value: { cust_id: 1040, amount: 12 },
  headers: [{ key: 'trace', value: 't1' }],
  raw: { key: 'k1', value: '{"cust_id":1040,"amount":12}', encoding: RawEncoding.Utf8 },
  decodeError: null,
};

const passed: EventResult = {
  seq: 0,
  status: EventStatus.Passed,
  error: null,
  branches: [
    {
      branch: 'main',
      status: EventStatus.Passed,
      filter: [{ path: '/branches/0/filter', result: true }],
      steps: [
        { index: 0, id: 's0', kind: 'rename', patch: [{ op: 'remove', path: '/value/cust_id' }, { op: 'add', path: '/value/customerId', value: 1040 }], durationUs: 1, logs: [], error: null },
        { index: 1, id: 's1', kind: 'set', patch: [{ op: 'add', path: '/value/vip', value: true }], durationUs: 1, logs: [], error: null },
      ],
      error: null,
      outputs: [],
    },
  ],
};

const filtered: EventResult = {
  seq: 0,
  status: EventStatus.Filtered,
  error: null,
  branches: [
    {
      branch: 'main',
      status: EventStatus.Filtered,
      filter: [
        { path: '/branches/0/filter/children/0', result: false },
        { path: '/branches/0/filter', result: false },
      ],
      steps: [],
      error: null,
      outputs: [],
    },
  ],
};

describe('StepProjection', () => {
  it('shows the record as read at the source step', () => {
    const p = StepProjection.project(record, passed, { step: BuilderStep.Source });
    expect(p.state).toBe(ProjectedState.Loaded);
    expect(p.document.value).toEqual(record.value);
  });

  it('shows filtered events at the filter step and after it', () => {
    expect(StepProjection.project(record, filtered, { step: BuilderStep.Filter }).state).toBe(ProjectedState.Filtered);
    expect(StepProjection.project(record, filtered, { step: BuilderStep.Destination }).state).toBe(ProjectedState.Filtered);
  });

  it('applies step patches up to the selected transform step', () => {
    const first = StepProjection.project(record, passed, { step: BuilderStep.Transform, stepIndex: 0 });
    expect(first.document.value).toEqual({ amount: 12, customerId: 1040 });
    expect(first.changed).toBe(true);
    const all = StepProjection.project(record, passed, { step: BuilderStep.Destination });
    expect(all.document.value).toEqual({ amount: 12, customerId: 1040, vip: true });
    expect(all.state).toBe(ProjectedState.Passed);
  });

  it('reports errors only from the failing step on', () => {
    const failing: EventResult = { ...passed, status: EventStatus.Error, error: { stepIndex: 1, step: 'set', errorType: 'step', message: 'boom' } };
    expect(StepProjection.project(record, failing, { step: BuilderStep.Transform, stepIndex: 0 }).state).toBe(ProjectedState.Passed);
    const at = StepProjection.project(record, failing, { step: BuilderStep.Transform, stepIndex: 1 });
    expect(at.state).toBe(ProjectedState.Error);
    expect(at.reason).toBe('set: boom');
  });

  it('marks decode errors at every step after the source', () => {
    const broken = { ...record, decodeError: 'invalid JSON' };
    expect(StepProjection.project(broken, filtered, { step: BuilderStep.Filter }).reason).toBe('invalid JSON');
  });
});

describe('FilterTrace', () => {
  it('maps node pointers back to visual rows; unevaluated rows are null', () => {
    expect(FilterTrace.rowResults(filtered.branches[0], 2)).toEqual([false, null]);
    expect(FilterTrace.rowResults(passed.branches[0], 1)).toEqual([true]);
    expect(FilterTrace.firstFailed([true, null, false])).toBe(2);
  });
});

describe('SampleSummary', () => {
  const summary: RunSummary = { loaded: 500, processed: 500, passed: 37, filtered: 461, errors: 2, dropped: 0, incomplete: false, durationMs: 5 };

  it('formats the counts line', () => {
    expect(SampleSummary.label(summary)).toBe('500 loaded · 37 pass · 461 filtered · 2 errors');
    expect(SampleSummary.label({ ...summary, dropped: 3 })).toBe('500 loaded · 37 pass · 461 filtered · 2 errors · 3 dropped');
  });

  it('explains incomplete runs and truncated or empty loads', () => {
    expect(SampleSummary.incompleteNotice({ ...summary, loaded: 1000, processed: 640, incomplete: true })).toBe('Previewed 640 of 1,000 events (time limit).');
    const session = { id: 's', expiresAt: '', loaded: 1000, bytes: 1, truncated: true, stopReason: StopReason.MaxEvents, masked: false };
    expect(SampleSummary.sessionNotice(session)).toBe('Showing first 1,000 events (limit).');
    expect(SampleSummary.sessionNotice({ ...session, loaded: 0, truncated: false })).toBe(SampleSummary.emptyNotice);
  });
});
