import { describe, expect, it } from 'vitest';
import { DeadLetterMapper } from '../../../api/http/deadLetters/DeadLetterMapper';
import type { DeadLetterItemDto, RetryReportDto } from '../../../api/http/deadLetters/DeadLetterDtos';
import { MockBackend } from '../../../api/mock/MockBackend';
import { MockDeadLettersService } from '../../../api/mock/MockDeadLettersService';
import { ClusterKind, DeadLetterDelivery, DeliveryMode, type Connection, type Route } from '../../../domain';
import { DeadLetterToasts } from '../../../store/DeadLetterActions';
import { AuditFilter } from '../../audit/model/AuditFilter';
import { AuditPresetId, AuditPresets } from '../../audit/model/AuditPresets';
import { DemoDeadLetters } from './DemoDeadLetters';

const item: DeadLetterItemDto = {
  id: 'x1', routeId: 'r1', routeName: 'orders', failedAt: '2026-10-14T10:00:00Z', errorType: 'step_error', message: 'boom', step: 's1',
  branch: 'main', versionNo: 2, source: { topic: 'orders', partition: 0, offset: 7 }, resolution: null, deliveryMode: 'exactly_once',
};

describe('exactly-once retries (DL-204)', () => {
  it('maps the route delivery mode and the retry flag', () => {
    expect(DeadLetterMapper.item(item).deliveryMode).toBe(DeliveryMode.ExactlyOnce);
    expect(DeadLetterMapper.item({ ...item, deliveryMode: undefined }).deliveryMode).toBeUndefined();
    const dto: RetryReportDto = { activeVersionNo: 2, results: [{ id: 'x1', outcome: 'sent' }], counts: {}, remaining: 0, deliveredAtLeastOnce: true };
    const report = DeadLetterMapper.retry(dto);
    expect(report.deliveredAtLeastOnce).toBe(true);
    expect(DeadLetterMapper.retry({ ...dto, deliveredAtLeastOnce: false }).deliveredAtLeastOnce).toBeUndefined();
    expect(DeadLetterMapper.merge([DeadLetterMapper.retry({ ...dto, deliveredAtLeastOnce: false }), report]).deliveredAtLeastOnce).toBe(true);
    expect(DeadLetterToasts.retried(report)).toContain('delivered at-least-once');
  });

  it('flags only exactly-once rows', () => {
    expect(DeadLetterDelivery.exactlyOnce([{ deliveryMode: DeliveryMode.AtLeastOnce }, {}])).toBe(false);
    expect(DeadLetterDelivery.exactlyOnce([{ deliveryMode: DeliveryMode.ExactlyOnce }])).toBe(true);
  });

  it('mock retries of exactly-once routes report at-least-once delivery', async () => {
    const service = new MockDeadLettersService(new MockBackend());
    const rows = (await service.page({ kinds: [] })).items;
    const eo = rows.find((d) => d.deliveryMode === DeliveryMode.ExactlyOnce);
    const plain = rows.find((d) => !d.deliveryMode);
    expect(eo && plain).toBeTruthy();
    expect((await service.retry([plain!.id])).deliveredAtLeastOnce).toBeUndefined();
    expect((await service.retry([eo!.id])).deliveredAtLeastOnce).toBe(true);
  });
});

describe('demo dead letters (DL-205)', () => {
  const demo = { id: 'c-demo', kind: ClusterKind.Demo } as Connection;
  const real = { id: 'c-real', kind: ClusterKind.Kafka } as Connection;
  const route = { id: 'r-demo', name: 'order-events', source: { connectionId: 'c-demo', topics: ['order-events'] } } as Route;
  const other = { id: 'r-other', name: 'other', source: { connectionId: 'c-real', topics: ['x'] } } as Route;

  it('finds the route reading the demo cluster and links to its dead letters', () => {
    expect(DemoDeadLetters.route([real], [other, route])).toBeUndefined();
    expect(DemoDeadLetters.route([real, demo], [other, route])?.id).toBe('r-demo');
    expect(DemoDeadLetters.path(route)).toBe('/dead-letters?route=r-demo');
    const summary = { unresolved: 12, approximate: false, byKind: [], routes: [{ routeId: 'r-demo', routeName: 'order-events', unresolved: 12, byKind: [] }] };
    expect(DemoDeadLetters.label(route, summary)).toContain('12 failed events on order-events');
  });
});

describe('audit preset "Dead-letter actions" (DL-206)', () => {
  const [preset] = AuditPresets.all;

  it('filters on the dlq object type and toggles back', () => {
    const on = AuditPresets.toggle({ ...AuditFilter.initial, action: 'route.deployed', teamId: 't1' }, preset);
    expect(on).toMatchObject({ objectType: 'dlq', action: '', teamId: 't1' });
    expect(AuditPresets.active(on, preset)).toBe(true);
    expect(AuditFilter.toQuery(on, new Date()).objectType).toBe('dlq');
    const off = AuditPresets.toggle(on, preset);
    expect(off).toEqual({ ...AuditFilter.initial, teamId: 't1' });
  });

  it('is preselected by ?preset=dlq', () => {
    expect(AuditPresets.initial(AuditPresetId.DeadLetters).objectType).toBe('dlq');
    expect(AuditPresets.initial('nope')).toEqual(AuditFilter.initial);
    expect(AuditPresets.path(AuditPresetId.DeadLetters)).toBe('/audit?preset=dlq');
  });
});
