import { describe, expect, it } from 'vitest';
import { LiveTailService, type TailChannel, type TailHandlers } from '../../../api/contracts';
import {
  TailCloseCode,
  TailFrameType,
  TailOp,
  TailOutcome,
  TailShow,
  TailState,
  type TailClientOp,
  type TailRecord,
} from '../../../domain';
import { TailBuffer } from './TailBuffer';
import { TailBackoff, TailSocketClient, type TailTimers } from './TailSocketClient';

/** Fake service: records every opened channel and the ops sent on it. */
class FakeTailService extends LiveTailService {
  readonly opened: { handlers: TailHandlers; sent: TailClientOp[]; closed: boolean }[] = [];

  open(_routeId: string, handlers: TailHandlers): TailChannel {
    const entry = { handlers, sent: [] as TailClientOp[], closed: false };
    this.opened.push(entry);
    return { send: (op) => (entry.sent.push(op), true), close: () => (entry.closed = true) };
  }
}

/** Manual timers: run() fires the pending callback. */
class ManualTimers implements TailTimers {
  pending: { fn: () => void; ms: number } | null = null;
  set = (fn: () => void, ms: number) => (this.pending = { fn, ms });
  clear = () => (this.pending = null);
  run(): void {
    const p = this.pending;
    this.pending = null;
    p?.fn();
  }
}

const record = (outcome: TailRecord['outcome'], email = 'a@b.c'): TailRecord => ({
  at: '2026-10-14T10:00:00Z', partition: 0, offset: 1, outcome, input: { partition: 0, offset: 1, encoding: 'json', value: { email } }, outputs: [],
});

function setup() {
  const service = new FakeTailService();
  const timers = new ManualTimers();
  const states: TailState[] = [];
  const records: TailRecord[] = [];
  const client = new TailSocketClient(service, 'r1', { onRecord: (r) => records.push(r), onState: (s) => states.push(s) }, new TailBackoff(), timers);
  return { service, timers, states, records, client };
}

describe('TailSocketClient', () => {
  it('forwards frames and reports states', () => {
    const { service, states, records, client } = setup();
    client.start();
    service.opened[0].handlers.onOpen?.();
    service.opened[0].handlers.onFrame({ type: TailFrameType.Status, state: TailState.Live, masked: true });
    service.opened[0].handlers.onFrame({ type: TailFrameType.Record, record: record(TailOutcome.Sent), masked: true });
    expect(states).toEqual([TailState.Connecting, TailState.Live]);
    expect(records).toHaveLength(1);
  });

  it('reconnects with backoff and re-applies pause and filter after reconnecting', () => {
    const { service, timers, states, client } = setup();
    client.start();
    client.setFilter({ show: TailShow.Errors, text: 'bob' });
    client.setPaused(true);
    service.opened[0].handlers.onClose(1006);
    expect(states.at(-1)).toBe(TailState.Reconnecting);
    expect(timers.pending?.ms).toBe(TailBackoff.initialMs);
    timers.run();
    service.opened[1].handlers.onOpen?.();
    expect(service.opened[1].sent).toEqual([
      { op: TailOp.Filter, outcomes: [TailOutcome.Error, TailOutcome.DeadLettered], text: 'bob' },
      { op: TailOp.Pause },
    ]);
  });

  it('does not reconnect when the route is not running, and stop closes the socket', () => {
    const { service, timers, states, client } = setup();
    client.start();
    service.opened[0].handlers.onClose(TailCloseCode.NotRunning);
    expect(states.at(-1)).toBe(TailState.NotRunning);
    expect(timers.pending).toBeNull();
    client.start();
    client.stop();
    expect(service.opened[1].closed).toBe(true);
    service.opened[1].handlers.onClose(1000);
    expect(timers.pending).toBeNull();
  });

  it('backs off exponentially up to the cap', () => {
    const b = new TailBackoff();
    expect(b.delay(0)).toBe(500);
    expect(b.delay(3)).toBe(4000);
    expect(b.delay(20)).toBe(TailBackoff.maxMs);
  });
});

describe('TailBuffer', () => {
  it('keeps the newest 500 rows first', () => {
    let rows: ReturnType<typeof TailBuffer.add> = [];
    for (let i = 0; i < 510; i++) rows = TailBuffer.add(rows, { seq: i, record: record(TailOutcome.Sent), masked: false });
    expect(rows).toHaveLength(500);
    expect(rows[0].seq).toBe(509);
  });

  it('filters by show-only choice and text', () => {
    const rows = [
      { seq: 1, record: record(TailOutcome.Sent, 'ann@x'), masked: false },
      { seq: 2, record: record(TailOutcome.Filtered, 'bob@x'), masked: false },
      { seq: 3, record: record(TailOutcome.Error, 'bob@x'), masked: false },
    ];
    expect(TailBuffer.visible(rows, { show: TailShow.Dropped, text: '' }).map((r) => r.seq)).toEqual([2]);
    expect(TailBuffer.visible(rows, { show: TailShow.Errors, text: '' }).map((r) => r.seq)).toEqual([3]);
    expect(TailBuffer.visible(rows, { show: TailShow.All, text: 'BOB' }).map((r) => r.seq)).toEqual([2, 3]);
  });
});
