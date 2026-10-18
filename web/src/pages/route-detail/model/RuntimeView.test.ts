import { describe, expect, it } from 'vitest';
import { HaltReason, ObservedState, RunnerState, Tone, type RouteRuntime } from '../../../domain';
import { RuntimeView } from './RuntimeView';

const now = new Date('2026-10-14T06:30:00Z');
const runtime: RouteRuntime = {
  observedState: ObservedState.Error,
  runningVersionNo: 9,
  haltedAt: '2026-10-14T06:28:00Z',
  haltReason: HaltReason.ErrorPolicyPause,
  errorMessage: 'step error',
  lag: 1500,
  replicas: [{ workerId: 'worker-a', state: RunnerState.Error, versionNo: 8, lag: 3, reportedAt: '2026-10-14T06:29:00Z', errorMessage: 'boom' }],
};

describe('RuntimeView', () => {
  it('summarises state, version, lag and replicas', () => {
    expect(RuntimeView.state(runtime)).toEqual({ label: 'Error', tone: Tone.Err });
    expect(RuntimeView.meta(runtime)).toBe('Running v9 · lag 1,500 · 1 replica');
  });

  it('shows the halt reason label and message', () => {
    const halt = RuntimeView.halt(runtime, now);
    expect(halt?.reason).toBe('Paused by error policy');
    expect(halt?.message).toBe('step error');
    expect(RuntimeView.halt({ ...runtime, haltedAt: null }, now)).toBeNull();
  });

  it('maps replicas by worker with state tone and version', () => {
    const [row] = RuntimeView.rows(runtime, now);
    expect(row).toMatchObject({ worker: 'worker-a', label: 'Error', tone: Tone.Err, version: 'v8', lag: '3', error: 'boom' });
    expect(RuntimeView.meta({ ...runtime, runningVersionNo: null, replicas: [] })).toBe('No version running · lag 1,500 · 0 replicas');
  });
});
