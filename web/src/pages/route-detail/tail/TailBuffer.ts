import { TailLimits, TailShowRules, type TailFilter, type TailRecord } from '../../../domain';

export type TailRow = { seq: number; record: TailRecord; masked: boolean };

/** The visible tail: newest first, capped at 500 rows; filtering also runs locally for rows already shown. */
export class TailBuffer {
  static add(rows: TailRow[], row: TailRow, max: number = TailLimits.maxRows): TailRow[] {
    return [row, ...rows].slice(0, max);
  }

  static visible(rows: TailRow[], filter: TailFilter): TailRow[] {
    return rows.filter((r) => TailShowRules.matches(r.record, filter));
  }
}

/** Text shown for a payload in the compact row and the split view. */
export class TailPayloadText {
  static readonly maxSummary = 160;

  static summary(value: unknown): string {
    const text = typeof value === 'string' ? value : JSON.stringify(value);
    return text.length > TailPayloadText.maxSummary ? `${text.slice(0, TailPayloadText.maxSummary)}…` : text;
  }
}
