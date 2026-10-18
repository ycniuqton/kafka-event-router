/**
 * Display formatting shared by all screens (design `fmt` and time labels).
 * Absolute times are shown in UTC so mock data renders identically everywhere.
 */
export class Format {
  static readonly locale = 'en-US';
  static readonly timeZone = 'UTC';
  static readonly millionThreshold = 1e6;
  static readonly thousandThreshold = 1e4;
  static readonly ms = { minute: 60_000, hour: 3_600_000, day: 86_400_000, week: 604_800_000, month: 2_592_000_000 } as const;
  static readonly placeholder = '—';

  /** 1.84M, 48.2k, 2,310 — the design's compact number format. */
  static number(n: number): string {
    if (n >= Format.millionThreshold) return `${(n / 1e6).toFixed(2)}M`;
    if (n >= Format.thousandThreshold) return `${(n / 1e3).toFixed(1)}k`;
    return n.toLocaleString(Format.locale);
  }

  static rate(eventsPerSec: number): string {
    return `${Format.number(eventsPerSec)}/s`;
  }

  static percent(n: number): string {
    return `${n}%`;
  }

  /** "just now", "12 min ago", "2 h ago", "yesterday", "3 d ago", "2 w ago", then a date. */
  static relativeTime(iso: string | null | undefined, now: Date): string {
    if (!iso) return 'never';
    const then = new Date(iso);
    const diff = now.getTime() - then.getTime();
    const { minute, hour, day, week, month } = Format.ms;
    if (diff < minute) return 'just now';
    if (diff < hour) return `${Math.floor(diff / minute)} min ago`;
    if (diff < day) return Format.sameDay(then, now) ? `${Math.floor(diff / hour)} h ago` : 'yesterday';
    if (diff < week) return `${Math.floor(diff / day)} d ago`;
    if (diff < month) return `${Math.floor(diff / week)} w ago`;
    return Format.shortDate(iso);
  }

  /** 09:41:07 */
  static clockTime(iso: string): string {
    return new Date(iso).toISOString().slice(11, 19);
  }

  /** 10-13 09:44 (audit log). */
  static monthDayTime(iso: string): string {
    const d = new Date(iso).toISOString();
    return `${d.slice(5, 10)} ${d.slice(11, 16)}`;
  }

  /** Oct 13 · 09:32 (version history). */
  static versionDate(iso: string): string {
    return `${Format.shortDate(iso)} · ${new Date(iso).toISOString().slice(11, 16)}`;
  }

  /** Oct 13 */
  static shortDate(iso: string): string {
    return new Date(iso).toLocaleDateString(Format.locale, { month: 'short', day: '2-digit', timeZone: Format.timeZone });
  }

  private static sameDay(a: Date, b: Date): boolean {
    return a.toISOString().slice(0, 10) === b.toISOString().slice(0, 10);
  }
}
