import type { DeadLetter, DeadLetterKind } from './deadLetter';
import { DeadLetterClasses, type DeadLetterErrorClass } from './deadLetterClass';

/** One normalisation rule of a failure message: matches → placeholder (design 00001 §3.1, same order as the server). */
type NormalizationRule = { pattern: RegExp; placeholder: string };

/** Turns a failure message into its cause pattern: ids, numbers, offsets, positions and timestamps become placeholders. */
export class MessageNormalizer {
  static readonly maxRunes = 240;

  static readonly rules: NormalizationRule[] = [
    { pattern: /\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:\.\d+)?(?:Z|[+-]\d{2}:?\d{2})?/g, placeholder: '<ts>' },
    { pattern: /\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b/g, placeholder: '<uuid>' },
    { pattern: /\b[A-Za-z]+[_-](?=[A-Za-z0-9]*\d)[A-Za-z0-9]+\b/g, placeholder: '<id>' },
    { pattern: /\b(?=[0-9a-fA-F]*\d)(?=[0-9a-fA-F]*[a-fA-F])[0-9a-fA-F]{8,}\b/g, placeholder: '<hex>' },
    { pattern: /\b(partition|offset)\s+\d+/gi, placeholder: '$1 <n>' },
    { pattern: /\bp\d+@\d+\b/g, placeholder: 'p<n>@<n>' },
    { pattern: /-?\d+(?:\.\d+)?/g, placeholder: '<n>' },
    { pattern: /\s+/g, placeholder: ' ' },
  ];

  static normalize(message: string): string {
    const out = MessageNormalizer.rules.reduce((text, rule) => text.replace(rule.pattern, rule.placeholder), message).trim();
    return [...out].slice(0, MessageNormalizer.maxRunes).join('');
  }
}

/** Cause identity: route + step + error type + normalized message (mock-side hash; the server uses sha256). */
export class CauseFingerprint {
  static readonly separator = '\u001f';
  static readonly fnvOffset = 0x811c9dc5;
  static readonly fnvPrime = 0x01000193;
  static readonly hexWidth = 8;
  static readonly radix = 16;

  static of(routeId: string, step: string, kind: DeadLetterKind, pattern: string): string {
    const text = [routeId, step, kind, pattern].join(CauseFingerprint.separator);
    return CauseFingerprint.hash(text, CauseFingerprint.fnvOffset) + CauseFingerprint.hash([...text].reverse().join(''), CauseFingerprint.fnvOffset);
  }

  private static hash(text: string, seed: number): string {
    let h = seed;
    for (let i = 0; i < text.length; i++) h = Math.imul(h ^ text.charCodeAt(i), CauseFingerprint.fnvPrime) >>> 0;
    return h.toString(CauseFingerprint.radix).padStart(CauseFingerprint.hexWidth, '0');
  }

  static ofRow(d: DeadLetter): string {
    return d.fingerprint ?? CauseFingerprint.of(d.routeId, d.stepId ?? d.step, d.kind, MessageNormalizer.normalize(d.message));
  }
}

/** Hourly counts of a group over the query window. */
export type DeadLetterTrend = { since: string; bucketSeconds: number; counts: number[] };

/** One distinct cause: how often, since when, which versions and a sample event to open. */
export type DeadLetterGroup = {
  fingerprint: string;
  routeId: string;
  routeName: string;
  stepId: string;
  step: string;
  kind: DeadLetterKind;
  errorClass: DeadLetterErrorClass;
  pattern: string;
  count: number;
  firstSeenAt: string;
  lastSeenAt: string;
  versions: number[];
  sample: { id: string; message: string; failedAt: string };
  trend: DeadLetterTrend;
};

/** Groups of a scan; `approximate` / `truncated` = the server stopped at its scan budget. */
export type DeadLetterGroupPage = {
  items: DeadLetterGroup[];
  scanned: number;
  truncated: boolean;
  approximate: boolean;
  byClass: Partial<Record<DeadLetterErrorClass, number>>;
};

/** Builds group pages from rows (mock backend, tests) with the server's ordering and trend buckets. */
export class DeadLetterGroups {
  static readonly buckets = 24;
  static readonly bucketSeconds = 3600;
  static readonly msPerSecond = 1000;
  static readonly percent = 100;
  static readonly empty: DeadLetterGroupPage = { items: [], scanned: 0, truncated: false, approximate: false, byClass: {} };

  static build(rows: DeadLetter[], now: Date, routeName: (id: string) => string): DeadLetterGroupPage {
    const since = new Date(now.getTime() - DeadLetterGroups.buckets * DeadLetterGroups.bucketSeconds * DeadLetterGroups.msPerSecond);
    const byId = new Map<string, DeadLetterGroup>();
    rows.forEach((d) => DeadLetterGroups.add(byId, d, since, routeName));
    const items = DeadLetterGroups.sorted([...byId.values()]);
    const byClass: Partial<Record<DeadLetterErrorClass, number>> = {};
    items.forEach((g) => (byClass[g.errorClass] = (byClass[g.errorClass] ?? 0) + g.count));
    return { items, scanned: rows.length, truncated: false, approximate: false, byClass };
  }

  /** Count desc, then most recently seen. */
  static sorted(groups: DeadLetterGroup[]): DeadLetterGroup[] {
    return [...groups].sort((a, b) => b.count - a.count || b.lastSeenAt.localeCompare(a.lastSeenAt));
  }

  private static add(byId: Map<string, DeadLetterGroup>, d: DeadLetter, since: Date, routeName: (id: string) => string): void {
    const fingerprint = CauseFingerprint.ofRow(d);
    const g = byId.get(fingerprint) ?? DeadLetterGroups.start(d, fingerprint, since, routeName(d.routeId));
    g.count++;
    if (d.failedAt < g.firstSeenAt) g.firstSeenAt = d.failedAt;
    if (d.failedAt > g.lastSeenAt) {
      g.lastSeenAt = d.failedAt;
      g.sample = { id: d.id, message: d.message, failedAt: d.failedAt };
    }
    if (d.versionNo && !g.versions.includes(d.versionNo)) g.versions = [...g.versions, d.versionNo].sort((a, b) => a - b);
    const bucket = Math.floor((new Date(d.failedAt).getTime() - since.getTime()) / (DeadLetterGroups.bucketSeconds * DeadLetterGroups.msPerSecond));
    if (bucket >= 0 && bucket < DeadLetterGroups.buckets) g.trend.counts[bucket]++;
    byId.set(fingerprint, g);
  }

  private static start(d: DeadLetter, fingerprint: string, since: Date, routeName: string): DeadLetterGroup {
    return {
      fingerprint,
      routeId: d.routeId,
      routeName,
      stepId: d.stepId ?? d.step,
      step: d.step,
      kind: d.kind,
      errorClass: DeadLetterClasses.of(d),
      pattern: MessageNormalizer.normalize(d.message),
      count: 0,
      firstSeenAt: d.failedAt,
      lastSeenAt: d.failedAt,
      versions: [],
      sample: { id: d.id, message: d.message, failedAt: d.failedAt },
      trend: { since: since.toISOString(), bucketSeconds: DeadLetterGroups.bucketSeconds, counts: new Array<number>(DeadLetterGroups.buckets).fill(0) },
    };
  }

  /** Bar heights 0–100 of a trend (relative to its busiest bucket). */
  static bars(trend: DeadLetterTrend): number[] {
    const peak = Math.max(1, ...trend.counts);
    return trend.counts.map((c) => Math.round((c / peak) * DeadLetterGroups.percent));
  }
}
