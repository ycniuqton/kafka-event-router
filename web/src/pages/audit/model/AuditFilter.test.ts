import { describe, expect, it } from 'vitest';
import { AuditMapper } from '../../../api/http/audit/AuditDtos';
import { AuditFilter, AuditTimeRange } from './AuditFilter';

const now = new Date('2026-10-14T10:00:00.000Z');

/** The query parameters the HTTP service sends for a filter bar state. */
function params(filter: Partial<typeof AuditFilter.initial>): Record<string, string> {
  const query = AuditFilter.toQuery({ ...AuditFilter.initial, ...filter }, now);
  const out: Record<string, string> = {};
  Object.entries(AuditMapper.query(query, null)).forEach(([k, v]) => {
    if (v !== null && v !== undefined) out[k] = String(v);
  });
  return out;
}

describe('AuditFilterBar → query params', () => {
  it('sends only the page size for the initial state', () => {
    expect(params({})).toEqual({ limit: '50' });
    expect(AuditFilter.isFiltered(AuditFilter.initial)).toBe(false);
  });

  it('maps every filter to its server parameter', () => {
    expect(params({ text: ' orders ', actorId: 'u1', action: 'sample.viewed_raw', objectType: 'sample_session', teamId: 't1', sensitiveOnly: true }))
      .toEqual({ q: 'orders', actor: 'u1', action: 'sample.viewed_raw', objectType: 'sample_session', teamId: 't1', sensitive: 'true', limit: '50' });
    expect(AuditFilter.isFiltered({ ...AuditFilter.initial, teamId: 't1' })).toBe(true);
  });

  it('resolves relative ranges against now and custom ranges from local inputs', () => {
    expect(params({ range: AuditTimeRange.Day })).toEqual({ from: '2026-10-13T10:00:00.000Z', limit: '50' });
    expect(params({ range: AuditTimeRange.Hour }).from).toBe('2026-10-14T09:00:00.000Z');
    const custom = params({ range: AuditTimeRange.Custom, customFrom: '2026-10-01T08:00', customTo: '' });
    expect(new Date(custom.from).getTime()).toBe(new Date('2026-10-01T08:00').getTime());
    expect(custom.to).toBeUndefined();
    expect(params({ range: AuditTimeRange.Custom, customFrom: 'not a date' }).from).toBeUndefined();
  });
});
