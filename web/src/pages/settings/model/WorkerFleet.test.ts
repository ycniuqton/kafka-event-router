import { describe, expect, it } from 'vitest';
import { Tone, type WorkerInfo } from '../../../domain';
import { WorkerFleet } from './WorkerFleet';

const now = new Date('2026-10-14T06:30:00Z');
const base: WorkerInfo = { id: 'b', host: 'host-b', zone: 'z1', version: 'dev', capacity: 100, capacityUsed: 1.1, draining: false, alive: true, lastHeartbeatAt: '2026-10-14T06:29:55Z', routes: 1 };

describe('WorkerFleet', () => {
  it('lists live workers first with health, place and load', () => {
    const rows = WorkerFleet.rows([{ ...base, id: 'a', alive: false }, base, { ...base, id: 'c', draining: true, routes: 2 }], now);
    expect(rows.map((r) => r.id)).toEqual(['b', 'c', 'a']);
    expect(rows[0]).toMatchObject({ place: 'host-b · z1', tone: Tone.Ok, load: '1 route · 1.1/100 capacity' });
    expect(rows[1]).toMatchObject({ label: 'Draining', tone: Tone.Warn, load: '2 routes · 1.1/100 capacity' });
    expect(rows[2]).toMatchObject({ label: 'No heartbeat', tone: Tone.Err });
    expect(WorkerFleet.rows([{ ...base, host: 'b' }], now)[0].place).toBe('z1');
  });
});
