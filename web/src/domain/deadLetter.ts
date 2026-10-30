import { DeliveryMode } from './routeSettings';
import type { DeadLetterErrorClass } from './deadLetterClass';
import { Tone } from './tone';

/** Why an event was sent to the dead-letter topic. */
export const DeadLetterKind = {
  TransformError: 'Transform error',
  Timeout: 'Timeout',
  InvalidOutput: 'Invalid output',
  DecodeError: 'Decode error',
  Unreadable: 'Unreadable record',
  LookupUnavailable: 'Lookup unavailable',
  DependencyUnavailable: 'Dependency unavailable',
  ProduceRetriable: 'Produce retriable',
  InternalError: 'Internal error',
  SandboxCrash: 'Sandbox crash',
  EncodeError: 'Encode error',
  ProduceError: 'Produce error',
  ConfigError: 'Config error',
} as const;
export type DeadLetterKind = (typeof DeadLetterKind)[keyof typeof DeadLetterKind];

/** How a dead letter was resolved (P2 "show resolved"). */
export const DeadLetterResolutionState = { Retried: 'retried', Filtered: 'filtered', Discarded: 'discarded' } as const;
export type DeadLetterResolutionState = (typeof DeadLetterResolutionState)[keyof typeof DeadLetterResolutionState];

/** Badge label and tone of each resolution. */
export const DeadLetterResolutionMeta: Record<DeadLetterResolutionState, { label: string; tone: Tone }> = {
  [DeadLetterResolutionState.Retried]: { label: 'Retried', tone: Tone.Ok },
  [DeadLetterResolutionState.Filtered]: { label: 'Filtered on retry', tone: Tone.Off },
  [DeadLetterResolutionState.Discarded]: { label: 'Discarded', tone: Tone.Warn },
};

/** Who resolved a dead letter, when and how (`by` is a user id; `jobId` when a bulk job did it). */
export type DeadLetterResolution = { state: DeadLetterResolutionState; by: string; at: string; jobId?: string };

/** A failed event (design `DLS`); `sampleId` points at the original sample event (mock data only, -1 otherwise). */
export type DeadLetter = {
  id: string;
  failedAt: string;
  kind: DeadLetterKind;
  message: string;
  stack: string;
  offset: number;
  partition: number;
  routeId: string;
  step: string;
  sampleId: number;
  routeName?: string;
  versionNo?: number;
  /** Step id written by the worker (filter value); `step` is its display label. */
  stepId?: string;
  resolution?: DeadLetterResolution | null;
  /** Delivery mode of the route's active version (P3: retries of exactly-once routes are at-least-once). */
  deliveryMode?: DeliveryMode;
  /** Whose fault (server-classified; derived from `kind` when absent). */
  errorClass?: DeadLetterErrorClass;
  /** Cause fingerprint (route + step + error type + normalized message). */
  fingerprint?: string;
  /** Delayed attempts an auto-retry policy used before parking the event (0 = none). */
  attempts?: number;
};

/** The original record of a dead letter, decoded (JSON, text, Avro/Protobuf via the schema registry, or base64 for raw bytes);
 * `masked` = values hidden for this viewer (types kept). */
export type DeadLetterOriginal = {
  /** Wire encodings of key / value (`json`, `utf8`, `base64`, `avro`, `protobuf`); edits are sent back in the same encoding. */
  keyEncoding?: string;
  valueEncoding?: string;
  key: unknown;
  value: unknown;
  headers: { key: string; value: string }[];
  rawBytes: boolean;
  masked?: boolean;
  schema?: { format: string; schemaId?: number };
};

/** Detail of a dead letter loaded on selection: stack trace and the original event. */
export type DeadLetterDetail = { stack: string; original: DeadLetterOriginal | null };

/** Why an action did not touch an event. */
export const DeadLetterSkipReason = {
  Locked: 'locked',
  AlreadyResolved: 'already_resolved',
  Expired: 'expired',
  NotFound: 'not_found',
  NotDiscarded: 'not_discarded',
} as const;
export type DeadLetterSkipReason = (typeof DeadLetterSkipReason)[keyof typeof DeadLetterSkipReason];

/** Labels of skip reasons in the retry report. */
export const DeadLetterSkipReasonLabel: Record<DeadLetterSkipReason, string> = {
  [DeadLetterSkipReason.Locked]: 'being retried by someone else',
  [DeadLetterSkipReason.AlreadyResolved]: 'already resolved',
  [DeadLetterSkipReason.Expired]: 'expired (past retention)',
  [DeadLetterSkipReason.NotFound]: 'not found',
  [DeadLetterSkipReason.NotDiscarded]: 'not discarded',
};

/** Outcome of retry / discard: `resolved` leave the queue, `failed` stay with their new error. */
export type DeadLetterActionReport = {
  resolved: string[];
  sent: number;
  filtered: number;
  failed: { id: string; message: string }[];
  skipped: { id: string; reason: DeadLetterSkipReason }[];
  remaining: number;
  /** The route is exactly-once, but the retried events were produced outside its transaction (at-least-once). */
  deliveredAtLeastOnce?: boolean;
  /** Events of routes whose replay needs a second person (strict 4-eyes on prod destinations); nothing was sent. */
  approvalRequired?: DeadLetterApprovalRequest[];
};

/** Events of one route that can only be replayed through an approved job. */
export type DeadLetterApprovalRequest = { routeId: string; ids: string[] };

/** Retry guarantee of exactly-once routes (FR-21): retries run outside the consumer transaction. */
export class DeadLetterDelivery {
  static readonly title = 'Retries are delivered at-least-once';
  static readonly notice =
    'This route delivers exactly once, but a retry produces outside the route transaction: if a retry is interrupted after the send, the event can reach the destination twice.';

  static exactlyOnce(rows: { deliveryMode?: DeliveryMode }[]): boolean {
    return rows.some((d) => d.deliveryMode === DeliveryMode.ExactlyOnce);
  }
}

/** A downloadable export. */
export type DeadLetterExportFile = { fileName: string; blob: Blob };

/** Where "Open in Transform Studio" goes: the server's builder URL, else the route builder at `sampleId`. */
export type DeadLetterStudioLink = { routeId: string; sampleId: number; sessionId: string | null; builderUrl: string | null };

/** Report rules shared by mock and REST services. */
export class DeadLetterReports {
  /** Events a sync call handles; larger selections run as background jobs. */
  static readonly syncCap = 500;

  /** Every id resolved; `delivered` counts them as sent (retry) rather than discarded. */
  static allResolved(ids: string[], delivered: boolean): DeadLetterActionReport {
    return { resolved: ids, sent: delivered ? ids.length : 0, filtered: 0, failed: [], skipped: [], remaining: 0 };
  }

  /** Ids that left the queue, including ones another user already resolved. */
  static gone(report: DeadLetterActionReport): string[] {
    const already = report.skipped.filter((s) => s.reason === DeadLetterSkipReason.AlreadyResolved || s.reason === DeadLetterSkipReason.Expired);
    return [...report.resolved, ...already.map((s) => s.id)];
  }
}

/** Time windows of the dead-letter filter (DLQ retention is 7 days). */
export const DeadLetterTimeRange = { All: 'all', Hour: '1h', Day: '24h', Week: '7d' } as const;
export type DeadLetterTimeRange = (typeof DeadLetterTimeRange)[keyof typeof DeadLetterTimeRange];

/** Labels and lengths of the time windows. */
export class DeadLetterTimeRanges {
  static readonly labels: Record<DeadLetterTimeRange, string> = {
    [DeadLetterTimeRange.All]: 'Any time',
    [DeadLetterTimeRange.Hour]: 'Last hour',
    [DeadLetterTimeRange.Day]: 'Last 24 h',
    [DeadLetterTimeRange.Week]: 'Last 7 days',
  };

  static readonly millis: Record<DeadLetterTimeRange, number | null> = {
    [DeadLetterTimeRange.All]: null,
    [DeadLetterTimeRange.Hour]: 3_600_000,
    [DeadLetterTimeRange.Day]: 86_400_000,
    [DeadLetterTimeRange.Week]: 604_800_000,
  };

  static parse(raw: string | null): DeadLetterTimeRange {
    return (Object.values(DeadLetterTimeRange) as string[]).includes(raw ?? '') ? (raw as DeadLetterTimeRange) : DeadLetterTimeRange.All;
  }

  /** ISO start of the window ending at `now`, or undefined for any time. */
  static since(range: DeadLetterTimeRange, now: Date): string | undefined {
    const ms = DeadLetterTimeRanges.millis[range];
    return ms === null ? undefined : new Date(now.getTime() - ms).toISOString();
  }
}

/** Which dead letters a list, export or "retry all" covers; no route = every readable route.
 * P2: failing step id, route version, message text and resolved rows. */
export type DeadLetterFilter = {
  routeId?: string;
  kinds: DeadLetterKind[];
  since?: string;
  until?: string;
  stepId?: string;
  versionNo?: number;
  text?: string;
  includeResolved?: boolean;
  /** Error classes (data / router / transient) and one cause fingerprint. */
  classes?: DeadLetterErrorClass[];
  fingerprint?: string;
};

/** Export file formats (P2: NDJSON). */
export const DeadLetterFileFormat = { Json: 'json', Ndjson: 'ndjson' } as const;
export type DeadLetterFileFormat = (typeof DeadLetterFileFormat)[keyof typeof DeadLetterFileFormat];

/** Labels of export formats. */
export const DeadLetterFileFormatLabel: Record<DeadLetterFileFormat, string> = {
  [DeadLetterFileFormat.Json]: 'JSON',
  [DeadLetterFileFormat.Ndjson]: 'NDJSON',
};

/** Background bulk actions (any size). */
export const DeadLetterJobAction = { Retry: 'retry', Discard: 'discard' } as const;
export type DeadLetterJobAction = (typeof DeadLetterJobAction)[keyof typeof DeadLetterJobAction];

/** Lifecycle of a bulk job. */
export const DeadLetterJobStatus = {
  Queued: 'queued',
  Running: 'running',
  CancelRequested: 'cancel_requested',
  Cancelled: 'cancelled',
  Succeeded: 'succeeded',
  Failed: 'failed',
  PendingApproval: 'pending_approval',
  Rejected: 'rejected',
} as const;
export type DeadLetterJobStatus = (typeof DeadLetterJobStatus)[keyof typeof DeadLetterJobStatus];

/** A background retry/discard over every event matching a filter. */
export type DeadLetterJob = {
  id: string;
  routeId: string;
  routeName: string;
  action: DeadLetterJobAction;
  status: DeadLetterJobStatus;
  total: number;
  processed: number;
  counts: Record<string, number>;
  error: string | null;
  createdAt: string;
  finishedAt: string | null;
  /** Events per second the job replays at. */
  ratePerSecond?: number;
  note?: string;
  /** Replay of one edited record. */
  edited?: boolean;
  /** Size of an explicit id selection (0 / absent for a filter). */
  idsCount?: number;
  requestedBy?: string;
  /** The caller requested it (never approves it: strict 4-eyes). */
  requestedByMe?: boolean;
  approval?: DeadLetterJobApproval;
};

/** 4-eyes state of a replay job. */
export type DeadLetterJobApproval = { required: boolean; decidedBy?: string; decidedAt?: string; decisionNote?: string };

/** Job rules shared by services, store and screens. */
export class DeadLetterJobs {
  static readonly terminal: DeadLetterJobStatus[] = [DeadLetterJobStatus.Cancelled, DeadLetterJobStatus.Succeeded, DeadLetterJobStatus.Failed, DeadLetterJobStatus.Rejected];
  static readonly pollMs = 2000;
  static readonly percentScale = 100;

  static done(job: DeadLetterJob): boolean {
    return DeadLetterJobs.terminal.includes(job.status);
  }

  /** Waits for a second person; nothing runs until it is approved. */
  static awaitingApproval(job: DeadLetterJob): boolean {
    return job.status === DeadLetterJobStatus.PendingApproval;
  }

  /** Progress 0–100 (0 while the total is unknown). */
  static percent(job: DeadLetterJob): number {
    return job.total > 0 ? Math.min(DeadLetterJobs.percentScale, Math.round((job.processed / job.total) * DeadLetterJobs.percentScale)) : 0;
  }

  static label(job: DeadLetterJob): string {
    const verb = job.action === DeadLetterJobAction.Retry ? 'Retry' : 'Discard';
    const total = job.total > 0 ? ` of ${job.total}` : '';
    return `${verb} of ${job.routeName || job.routeId}: ${job.processed}${total} events · ${job.status.replace('_', ' ')}`;
  }
}

/** One page of a dead-letter scan; `truncated` = the server hit its scan budget before filling the page. */
export type DeadLetterPage = { items: DeadLetter[]; nextCursor: string | null; truncated: boolean };

/** Unresolved count per error kind. */
export type DeadLetterKindCount = { kind: DeadLetterKind; count: number };

/** Unresolved dead letters of one route. */
export type DeadLetterRouteCount = { routeId: string; routeName: string; unresolved: number; byKind: DeadLetterKindCount[] };

/** Unresolved counts for the nav badge, route tab label, dashboard card and filter chips (approximate on large queues). */
export type DeadLetterSummary = {
  unresolved: number;
  approximate: boolean;
  byKind: DeadLetterKindCount[];
  routes: DeadLetterRouteCount[];
  /** Unresolved per error class (absent from older servers: derive from `byKind`). */
  byClass?: Partial<Record<DeadLetterErrorClass, number>>;
};

/** Export selection: explicit ids, or everything matching a filter. */
export type DeadLetterSelection = { ids: string[] } | { filter: DeadLetterFilter };

/** Summary rules shared by mock and REST services and the screens. */
export class DeadLetterSummaries {
  static readonly empty: DeadLetterSummary = { unresolved: 0, approximate: false, byKind: [], routes: [] };

  static route(summary: DeadLetterSummary, routeId: string): DeadLetterRouteCount | undefined {
    return summary.routes.find((r) => r.routeId === routeId);
  }

  /** Counts per kind of a list, in first-seen order. */
  static kinds(list: { kind: DeadLetterKind }[]): DeadLetterKindCount[] {
    const counts = new Map<DeadLetterKind, number>();
    list.forEach((d) => counts.set(d.kind, (counts.get(d.kind) ?? 0) + 1));
    return [...counts].map(([kind, count]) => ({ kind, count }));
  }
}
