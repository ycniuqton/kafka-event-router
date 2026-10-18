import { AnyAuditQuery, type AuditQuery } from '../../../domain';

/** Time range presets of the filter bar; `custom` uses the two date-time inputs. */
export const AuditTimeRange = { Any: 'any', Hour: '1h', Day: '24h', Week: '7d', Month: '30d', Custom: 'custom' } as const;
export type AuditTimeRange = (typeof AuditTimeRange)[keyof typeof AuditTimeRange];

/** Filter bar state: the server filters plus the time range choice (custom bounds in local `datetime-local` form). */
export type AuditFilterState = {
  text: string;
  actorId: string;
  action: string;
  objectType: string;
  teamId: string;
  sensitiveOnly: boolean;
  range: AuditTimeRange;
  customFrom: string;
  customTo: string;
};

/** Initial filters, option labels and the debounce of free-text search. */
export class AuditFilter {
  static readonly any = '';
  static readonly initial: AuditFilterState = {
    text: '', actorId: '', action: '', objectType: '', teamId: '', sensitiveOnly: false, range: AuditTimeRange.Any, customFrom: '', customTo: '',
  };
  static readonly textDebounceMs = 300;
  static readonly rangeLabels: Record<AuditTimeRange, string> = {
    [AuditTimeRange.Any]: 'Any time',
    [AuditTimeRange.Hour]: 'Last hour',
    [AuditTimeRange.Day]: 'Last 24 hours',
    [AuditTimeRange.Week]: 'Last 7 days',
    [AuditTimeRange.Month]: 'Last 30 days',
    [AuditTimeRange.Custom]: 'Custom range',
  };
  private static readonly hourMs = 3_600_000;
  private static readonly rangeMs: Partial<Record<AuditTimeRange, number>> = {
    [AuditTimeRange.Hour]: AuditFilter.hourMs,
    [AuditTimeRange.Day]: 24 * AuditFilter.hourMs,
    [AuditTimeRange.Week]: 7 * 24 * AuditFilter.hourMs,
    [AuditTimeRange.Month]: 30 * 24 * AuditFilter.hourMs,
  };

  static isFiltered(filter: AuditFilterState): boolean {
    return JSON.stringify(filter) !== JSON.stringify(AuditFilter.initial);
  }

  /** The server query of the bar state; relative ranges are resolved against `now` once, when the filter changes. */
  static toQuery(filter: AuditFilterState, now: Date): AuditQuery {
    const { from, to } = AuditFilter.bounds(filter, now);
    return {
      ...AnyAuditQuery,
      text: filter.text,
      actorId: filter.actorId,
      sensitiveOnly: filter.sensitiveOnly,
      actions: filter.action ? [filter.action] : [],
      objectType: filter.objectType,
      teamId: filter.teamId,
      from,
      to,
    };
  }

  private static bounds(filter: AuditFilterState, now: Date): { from: string | null; to: string | null } {
    if (filter.range === AuditTimeRange.Custom) {
      return { from: AuditFilter.iso(filter.customFrom), to: AuditFilter.iso(filter.customTo) };
    }
    const span = AuditFilter.rangeMs[filter.range];
    return { from: span ? new Date(now.getTime() - span).toISOString() : null, to: null };
  }

  /** `datetime-local` value → ISO instant (empty or invalid → open bound). */
  private static iso(local: string): string | null {
    const t = local ? new Date(local).getTime() : Number.NaN;
    return Number.isNaN(t) ? null : new Date(t).toISOString();
  }
}

/** Grid layout and row metrics of the audit table (fixed row height keeps the virtual window exact). */
export const AuditLayout = {
  columns: '110px 130px minmax(180px,1fr) minmax(200px,1.3fr) 110px',
  minWidth: 760,
  rowHeight: 41,
  viewportRows: 14,
  overscan: 6,
  loadAheadRows: 10,
} as const;
