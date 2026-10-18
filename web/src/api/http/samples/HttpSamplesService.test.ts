import { describe, expect, it } from 'vitest';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import { PreviewErrors } from '../preview/HttpPreviewClient';
import { PreviewProblemType, type LoadSamplesResponse, type SampleDto } from '../preview/PreviewDtos';
import { HttpSamplesService } from './HttpSamplesService';

function sample(seq: number, value: SampleDto['value']): SampleDto {
  return {
    seq,
    topic: 'order-events',
    partition: 0,
    offset: 100 + seq,
    timestamp: '2026-10-13T09:00:00Z',
    key: `o-${seq}`,
    value,
    headers: [{ key: 'source', value: 'web' }],
    raw: { key: `o-${seq}`, value: JSON.stringify(value), encoding: 'utf8' },
    decodeError: null,
  };
}

function loaded(events: SampleDto[]): LoadSamplesResponse {
  return { session: { id: 'ps_1', expiresAt: '', loaded: events.length, bytes: 1, truncated: false, stopReason: 'complete', masked: false }, events };
}

function setup() {
  const fetch = new TestFetch();
  let clock = 0;
  const service = new HttpSamplesService(new HttpContext({ apiBaseUrl: '/api/v1', fetch: fetch.fn }), () => clock);
  return { fetch, service, tick: (ms: number) => (clock += ms) };
}

describe('HttpSamplesService', () => {
  it('loads the latest events of a topic and maps them to sample events', async () => {
    const { fetch, service } = setup();
    fetch.json(201, loaded([sample(0, { eventType: 'OrderPaid', amount: 10 }), sample(1, 'plain')]));
    const events = await service.list('c1', 'order-events');
    expect(fetch.calls[0].url).toBe('/api/v1/preview/samples');
    expect(JSON.parse(String(fetch.calls[0].init.body))).toEqual({ connectionId: 'c1', topics: ['order-events'], mode: 'latest', count: 50 });
    expect(events[0]).toMatchObject({ id: 0, key: 'o-0', offset: 100, headers: { source: 'web' }, value: { eventType: 'OrderPaid' } });
    expect(events[1].value).toEqual({ value: 'plain' });
  });

  it('peeks with the requested count, newest first, and reuses a recent load', async () => {
    const { fetch, service, tick } = setup();
    fetch.json(201, loaded([sample(0, {}), sample(1, {})])).json(201, loaded([]));
    expect((await service.peek('c1', 't', 20)).map((e) => e.id)).toEqual([1, 0]);
    await service.peek('c1', 't', 20);
    expect(fetch.calls).toHaveLength(1);
    tick(61_000);
    await service.peek('c1', 't', 20);
    expect(fetch.calls).toHaveLength(2);
    expect(JSON.parse(String(fetch.calls[1].init.body)).count).toBe(20);
  });

  it('derives filter fields from the session schema', async () => {
    const { fetch, service } = setup();
    expect(await service.fields()).toEqual([]);
    fetch.json(201, loaded([sample(0, {})])).json(200, {
      samples: 1,
      root: {
        path: '', types: { object: 1 }, present: 1, frequency: 1, topValues: [],
        children: [
          { path: 'key', types: { string: 1 }, present: 1, frequency: 1, topValues: [], children: [] },
          { path: 'value', types: { object: 1 }, present: 1, frequency: 1, topValues: [], children: [
            { path: 'value.amount', types: { number: 3, string: 1 }, present: 1, frequency: 1, topValues: [], children: [] },
            { path: 'value.items[]', types: { object: 1 }, present: 1, frequency: 1, topValues: [], children: [] },
          ] },
        ],
      },
    });
    await service.list('c1', 't');
    expect(await service.fields()).toEqual([{ path: 'key', numeric: false }, { path: 'value.amount', numeric: true }]);
    expect(fetch.calls[1].url).toBe('/api/v1/preview/sessions/ps_1/schema');
  });

  it('recognises an expired session problem', async () => {
    const { fetch, service } = setup();
    fetch.json(201, loaded([])).problem(410, { type: PreviewProblemType.SessionExpired, title: 'Gone', status: 410 });
    await service.list('c1', 't');
    expect(await service.fields()).toEqual([]);
    const error = await TestFetch.failure(new HttpSamplesService(new HttpContext({ fetch: new TestFetch().problem(429, { type: PreviewProblemType.Busy, title: 'x', status: 429 }).fn })).list('c', 't'));
    expect(PreviewErrors.isBusy(error)).toBe(true);
  });
});
