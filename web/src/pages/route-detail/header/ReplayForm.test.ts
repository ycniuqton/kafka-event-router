import { describe, expect, it, vi } from 'vitest';
import { ApiError } from '../../../api/http/core';
import { ReplayStatus, type Replay, type ReplayRequest } from '../../../domain';
import { ReplayForm, ReplayText } from './ReplayForm';

const now = new Date('2026-10-14T12:00:00Z');
const valid = { start: '2026-10-14T10:30', acknowledged: true, typedName: 'orders-to-billing' };
const replay: Replay = { id: 'rp1', routeId: 'r1', status: ReplayStatus.Pending, from: '2026-10-14T10:30:00.000Z', requestedAt: now.toISOString(),
  startedAt: null, finishedAt: null, error: '', partitions: 0 };

describe('ReplayForm', () => {
  it('requires a past start, the duplicate acknowledgement and the exact route name', () => {
    expect(ReplayForm.validate(valid, now, 'orders-to-billing')).toEqual({});
    expect(ReplayForm.validate({ ...valid, start: '' }, now, 'orders-to-billing').from).toBe(ReplayText.fromRequired);
    expect(ReplayForm.validate({ ...valid, start: '2026-10-15T00:00' }, now, 'orders-to-billing').from).toBe(ReplayText.fromFuture);
    expect(ReplayForm.validate({ ...valid, acknowledged: false }, now, 'orders-to-billing').ack).toBe(ReplayText.ackRequired);
    expect(ReplayForm.validate({ ...valid, typedName: 'orders' }, now, 'orders-to-billing').name).toBe(ReplayText.nameMismatch);
  });

  it('reads the datetime-local value as UTC and confirms the duplicates', () => {
    expect(ReplayForm.request(valid)).toEqual({ from: '2026-10-14T10:30:00.000Z', confirm: true });
    expect(ReplayForm.defaultStart(now)).toBe('2026-10-14T11:00');
  });

  it('never calls the API while the form is invalid', async () => {
    const start = vi.fn<(r: ReplayRequest) => Promise<Replay>>();
    const outcome = await ReplayForm.submit({ ...valid, typedName: '' }, now, 'orders-to-billing', start);
    expect(start).not.toHaveBeenCalled();
    expect(outcome.errors?.name).toBe(ReplayText.nameMismatch);
  });

  it('calls RoutesService.replay with confirm and returns the started replay', async () => {
    const start = vi.fn<(r: ReplayRequest) => Promise<Replay>>(() => Promise.resolve(replay));
    const outcome = await ReplayForm.submit(valid, now, 'orders-to-billing', start);
    expect(start).toHaveBeenCalledWith({ from: '2026-10-14T10:30:00.000Z', confirm: true });
    expect(outcome.replay).toBe(replay);
  });

  it('maps a 422 problem on /from to the time field and other failures to the form', async () => {
    const problem = ApiError.fromProblem({ type: 'about:blank', title: 'Invalid', status: 422, detail: 'invalid',
      errors: [{ path: '/from', code: 'future', message: 'In the future' }] });
    const fieldOutcome = await ReplayForm.submit(valid, now, 'orders-to-billing', () => Promise.reject(problem));
    expect(fieldOutcome.errors).toEqual({ from: 'In the future' });
    const formOutcome = await ReplayForm.submit(valid, now, 'orders-to-billing', () => Promise.reject(new Error('A replay is already running')));
    expect(formOutcome.errors).toEqual({ form: 'A replay is already running' });
  });
});
