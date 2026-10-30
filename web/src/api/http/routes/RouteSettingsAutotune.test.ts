import { describe, expect, it } from 'vitest';
import { InFlightMode, RouteTuningText } from '../../../domain';
import type { RouteDetailDto } from './RouteDtos';
import { RouteSettingsMapper } from './RouteSettingsMapper';

function withLimits(limits: Record<string, unknown> | undefined): RouteDetailDto {
  const draft = { apiVersion: 'router/v1', name: 'orders', source: { connectionId: 'c1', topics: ['in'], format: { type: 'json' } }, branches: [], limits };
  return { id: 'r1', name: 'orders', replicas: 1, draft } as unknown as RouteDetailDto;
}

describe('RouteSettingsMapper autotune limits', () => {
  it('reads the in-flight mode apart from the numeric limits', () => {
    const s = RouteSettingsMapper.fromDetail(withLimits({ inFlightMode: 'fixed', maxInFlight: 5000, jsMaxInstances: 8 }));
    expect(s.inFlightMode).toBe(InFlightMode.Fixed);
    expect(s.limits).toMatchObject({ maxInFlight: 5000, jsMaxInstances: 8, minInFlight: null, jsMinInstances: null });
    expect(Object.keys(s.limits)).not.toContain('inFlightMode');
  });

  it('defaults to auto and writes only a fixed mode', () => {
    const dto = withLimits(undefined);
    const s = RouteSettingsMapper.fromDetail(dto);
    expect(s.inFlightMode).toBe(InFlightMode.Auto);
    const auto = RouteSettingsMapper.toBody(dto, { inFlightMode: InFlightMode.Auto, limits: { ...s.limits, minInFlight: 32 } });
    expect(auto.draft?.limits).toEqual({ minInFlight: 32 });
    const fixed = RouteSettingsMapper.toBody(dto, { inFlightMode: InFlightMode.Fixed });
    expect(fixed.draft?.limits).toEqual({ inFlightMode: 'fixed' });
  });
});

describe('RouteTuningText', () => {
  it('summarises the reported values and hides missing ones', () => {
    expect(RouteTuningText.summary({ inFlightLimit: 4200, jsInstances: 6, jsQueueWaitP95: 0.0012 })).toBe(
      'Now: in-flight limit 4,200 · JS instances 6 · JS queue wait p95 1.2 ms',
    );
    expect(RouteTuningText.summary({ inFlightLimit: 900, jsInstances: null, jsQueueWaitP95: null })).toBe('Now: in-flight limit 900');
    expect(RouteTuningText.summary({ inFlightLimit: null, jsInstances: null, jsQueueWaitP95: null })).toBeNull();
    expect(RouteTuningText.summary(undefined)).toBeNull();
  });
});
