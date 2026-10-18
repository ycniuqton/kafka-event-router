import { describe, expect, it } from 'vitest';
import { HaltReason, ObservedState, RunnerState } from '../../../domain';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import { HttpRuntimeService } from './HttpRuntimeService';

function setup() {
  const fetch = new TestFetch();
  return { fetch, service: new HttpRuntimeService(new HttpContext({ apiBaseUrl: '/api/v1', fetch: fetch.fn })) };
}

describe('HttpRuntimeService', () => {
  it('maps the runtime view with omitted optional members to nulls', async () => {
    const { fetch, service } = setup();
    fetch.json(200, {
      observedState: 'error',
      runningVersionNo: 3,
      haltedAt: '2026-10-13T09:00:00Z',
      haltReason: 'error_policy_pause',
      errorMessage: 'decode error',
      lag: 12,
      replicas: [{ workerId: 'worker-1', state: 'error', versionNo: 3, lag: 12, reportedAt: null }],
    });
    const rt = await service.get('r 1');
    expect(fetch.calls[0].url).toContain('/api/v1/routes/r%201/runtime');
    expect(rt.observedState).toBe(ObservedState.Error);
    expect(rt.haltReason).toBe(HaltReason.ErrorPolicyPause);
    expect(rt.replicas[0]).toEqual({ workerId: 'worker-1', state: RunnerState.Error, versionNo: 3, lag: 12, reportedAt: null, errorMessage: null });
  });

  it('pages through workers', async () => {
    const { fetch, service } = setup();
    const worker = { id: 'w1', host: 'h', zone: 'a', version: '1', capacity: 100, capacityUsed: 2, draining: false, alive: true, lastHeartbeatAt: '2026-10-13T09:00:00Z', routes: 2 };
    fetch.json(200, { items: [worker], next_cursor: 'c2' }).json(200, { items: [{ ...worker, id: 'w2' }], next_cursor: null });
    const workers = await service.workers();
    expect(workers.map((w) => w.id)).toEqual(['w1', 'w2']);
    expect(fetch.calls[1].url).toContain('cursor=c2');
  });
});
