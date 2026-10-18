import {
  DeadLetterDelivery,
  DeadLetterFileFormat,
  DeadLetterJobAction,
  DeadLetterJobStatus,
  DeadLetterJobs,
  DeadLetterReports,
  DeadLetterResolutionState,
  DeadLetterSkipReason,
  DeadLetterSummaries,
  type DeadLetter,
  type DeadLetterActionReport,
  type DeadLetterDetail,
  type DeadLetterExportFile,
  type DeadLetterFilter,
  type DeadLetterJob,
  type DeadLetterPage,
  type DeadLetterResolution,
  type DeadLetterSelection,
  type DeadLetterStudioLink,
  type DeadLetterSummary,
} from '../../domain';
import { SAMPLES } from '../../mocks';
import { DeadLettersService } from '../contracts';
import type { MockBackend } from './MockBackend';

/** Export file naming and media types shared with the REST export. */
export const DeadLetterExportFormat = {
  mimeType: 'application/json',
  ndjsonMimeType: 'application/x-ndjson',
  prefix: 'dead-letter-',
  indent: 2,
  lineBreak: '\n',
} as const;

/** Mock paging: page size and the cursor = index of the next row; jobs process `jobStep` events per poll. */
export const MockDeadLetterPaging = { pageSize: 50, syncCap: DeadLetterReports.syncCap, jobStep: 2, mockUser: 'u-me' } as const;

/** Filter rules of the in-memory queue (same semantics as the server scan). */
export class MockDeadLetterFilter {
  static matches(d: DeadLetter, f: DeadLetterFilter): boolean {
    if (f.routeId && d.routeId !== f.routeId) return false;
    if (f.kinds.length && !f.kinds.includes(d.kind)) return false;
    if (f.since && d.failedAt < f.since) return false;
    if (f.until && d.failedAt >= f.until) return false;
    return MockDeadLetterFilter.matchesP2(d, f);
  }

  /** Step id, route version and case-insensitive message text. */
  private static matchesP2(d: DeadLetter, f: DeadLetterFilter): boolean {
    if (f.stepId && (d.stepId ?? d.step) !== f.stepId) return false;
    if (f.versionNo && d.versionNo !== f.versionNo) return false;
    return !f.text || d.message.toLowerCase().includes(f.text.toLowerCase());
  }
}

/** Type-preserving masker of the mock (strings → bullets of the same length, numbers → 0, booleans → false). */
export class MockValueMasker {
  static readonly bullet = '•';

  static mask(v: unknown): unknown {
    if (Array.isArray(v)) return v.map((x) => MockValueMasker.mask(x));
    if (v !== null && typeof v === 'object') return Object.fromEntries(Object.entries(v).map(([k, x]) => [k, MockValueMasker.mask(x)]));
    if (typeof v === 'string') return MockValueMasker.bullet.repeat(v.length);
    if (typeof v === 'number') return 0;
    return typeof v === 'boolean' ? false : v;
  }
}

/** In-memory dead-letter queue: retry/discard resolve events (kept for "show resolved"), restore un-discards, jobs run per poll. */
export class MockDeadLettersService extends DeadLettersService {
  private readonly backend: MockBackend;
  private readonly resolutions = new Map<string, DeadLetterResolution>();
  private readonly jobs = new Map<string, { job: DeadLetterJob; pending: string[] }>();

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  page(filter: DeadLetterFilter, cursor?: string | null): Promise<DeadLetterPage> {
    const matching = this.matching(filter);
    const start = Number(cursor ?? 0);
    const end = start + MockDeadLetterPaging.pageSize;
    const items = matching.slice(start, end).map((d) => ({ ...d, routeName: this.routeName(d.routeId), resolution: this.resolutions.get(d.id) ?? null }));
    return this.backend.reply({ items, nextCursor: end < matching.length ? String(end) : null, truncated: false });
  }

  summary(routeId?: string): Promise<DeadLetterSummary> {
    const all = this.matching({ routeId, kinds: [] });
    const routeIds = [...new Set(all.map((d) => d.routeId))];
    const routes = routeIds.map((id) => {
      const own = all.filter((d) => d.routeId === id);
      return { routeId: id, routeName: this.routeName(id), unresolved: own.length, byKind: DeadLetterSummaries.kinds(own) };
    });
    return this.backend.reply({ unresolved: all.length, approximate: false, byKind: DeadLetterSummaries.kinds(all), routes });
  }

  /** Detail; events of routes reading a `sensitive` connection come back masked. */
  detail(deadLetter: DeadLetter): Promise<DeadLetterDetail> {
    const sample = SAMPLES.find((e) => e.id === deadLetter.sampleId);
    if (!sample) return this.backend.reply({ stack: deadLetter.stack, original: null });
    const masked = this.sensitive(deadLetter.routeId);
    const show = (v: unknown) => (masked ? MockValueMasker.mask(v) : v);
    return this.backend.reply({ stack: deadLetter.stack, original: { key: show(sample.key), value: show(sample.value), headers: [], rawBytes: false, masked } });
  }

  async retry(ids: string[]): Promise<DeadLetterActionReport> {
    await this.resolve(ids, DeadLetterResolutionState.Retried);
    const rows = this.backend.db.deadLetters.filter((d) => ids.includes(d.id));
    const report = DeadLetterReports.allResolved(ids, true);
    return DeadLetterDelivery.exactlyOnce(rows) ? { ...report, deliveredAtLeastOnce: true } : report;
  }

  async retryMatching(routeId: string, filter: DeadLetterFilter): Promise<DeadLetterActionReport> {
    const matching = this.matching({ ...filter, routeId, includeResolved: false });
    const ids = matching.slice(0, MockDeadLetterPaging.syncCap).map((d) => d.id);
    const report = await this.retry(ids);
    return { ...report, remaining: matching.length - ids.length };
  }

  async discard(ids: string[]): Promise<DeadLetterActionReport> {
    await this.resolve(ids, DeadLetterResolutionState.Discarded);
    return DeadLetterReports.allResolved(ids, false);
  }

  /** Un-discards; events resolved another way are skipped. */
  async restore(ids: string[]): Promise<DeadLetterActionReport> {
    const restorable = ids.filter((id) => this.resolutions.get(id)?.state === DeadLetterResolutionState.Discarded);
    restorable.forEach((id) => this.resolutions.delete(id));
    await this.backend.reply(null);
    const skipped = ids.filter((id) => !restorable.includes(id)).map((id) => ({ id, reason: DeadLetterSkipReason.NotDiscarded }));
    return { ...DeadLetterReports.allResolved(restorable, false), skipped };
  }

  async exportJson(routeId: string, selection: DeadLetterSelection, format: DeadLetterFileFormat = DeadLetterFileFormat.Json): Promise<DeadLetterExportFile> {
    const rows = 'ids' in selection ? this.backend.db.deadLetters.filter((d) => selection.ids.includes(d.id)) : this.matching({ ...selection.filter, routeId });
    const details = await Promise.all(rows.map(async (d) => ({ ...d, originalEvent: (await this.detail(d)).original })));
    const ndjson = format === DeadLetterFileFormat.Ndjson;
    const body = ndjson ? details.map((d) => JSON.stringify(d)).join(DeadLetterExportFormat.lineBreak) : JSON.stringify(details, null, DeadLetterExportFormat.indent);
    const type = ndjson ? DeadLetterExportFormat.ndjsonMimeType : DeadLetterExportFormat.mimeType;
    return { fileName: `${DeadLetterExportFormat.prefix}${routeId}.${format}`, blob: new Blob([body], { type }) };
  }

  openInStudio(deadLetter: DeadLetter): Promise<DeadLetterStudioLink> {
    return this.backend.reply({ routeId: deadLetter.routeId, sampleId: deadLetter.sampleId, sessionId: null, builderUrl: null });
  }

  startJob(routeId: string, action: DeadLetterJobAction, filter: DeadLetterFilter): Promise<DeadLetterJob> {
    const pending = this.matching({ ...filter, routeId, includeResolved: false }).map((d) => d.id);
    const job: DeadLetterJob = {
      id: `job-${this.jobs.size + 1}`,
      routeId,
      routeName: this.routeName(routeId),
      action,
      status: DeadLetterJobStatus.Queued,
      total: pending.length,
      processed: 0,
      counts: {},
      error: null,
      createdAt: this.backend.clock.now().toISOString(),
      finishedAt: null,
    };
    this.jobs.set(job.id, { job, pending });
    return this.backend.reply(job);
  }

  /** Each poll processes the next `jobStep` events (the mock's notion of time). */
  async job(jobId: string): Promise<DeadLetterJob> {
    const entry = this.jobs.get(jobId);
    if (!entry) return this.backend.fail(`Job ${jobId} does not exist.`);
    if (!DeadLetterJobs.done(entry.job)) await this.advance(entry);
    return this.backend.reply(entry.job);
  }

  cancelJob(jobId: string): Promise<DeadLetterJob> {
    const entry = this.jobs.get(jobId);
    if (!entry) return this.backend.fail(`Job ${jobId} does not exist.`);
    if (!DeadLetterJobs.done(entry.job)) entry.job = { ...entry.job, status: DeadLetterJobStatus.Cancelled, finishedAt: this.backend.clock.now().toISOString() };
    return this.backend.reply(entry.job);
  }

  private async advance(entry: { job: DeadLetterJob; pending: string[] }): Promise<void> {
    const batch = entry.pending.splice(0, MockDeadLetterPaging.jobStep);
    const state = entry.job.action === DeadLetterJobAction.Retry ? DeadLetterResolutionState.Retried : DeadLetterResolutionState.Discarded;
    await this.resolve(batch, state);
    const outcome = entry.job.action === DeadLetterJobAction.Retry ? 'sent' : 'discarded';
    const counts = { ...entry.job.counts, [outcome]: (entry.job.counts[outcome] ?? 0) + batch.length };
    const done = entry.pending.length === 0;
    const status = done ? DeadLetterJobStatus.Succeeded : DeadLetterJobStatus.Running;
    entry.job = { ...entry.job, counts, processed: entry.job.processed + batch.length, status, finishedAt: done ? this.backend.clock.now().toISOString() : null };
  }

  private matching(filter: DeadLetterFilter): DeadLetter[] {
    return this.backend.db.deadLetters.filter((d) => MockDeadLetterFilter.matches(d, filter) && (filter.includeResolved || !this.resolutions.has(d.id)));
  }

  private routeName(routeId: string): string {
    return this.backend.db.routes.find((r) => r.id === routeId)?.name ?? routeId;
  }

  private sensitive(routeId: string): boolean {
    const route = this.backend.db.routes.find((r) => r.id === routeId);
    return !!this.backend.db.connections.find((c) => c.id === route?.source.connectionId)?.sensitive;
  }

  private async resolve(ids: string[], state: DeadLetterResolutionState): Promise<void> {
    const at = this.backend.clock.now().toISOString();
    ids.filter((id) => !this.resolutions.has(id)).forEach((id) => this.resolutions.set(id, { state, by: MockDeadLetterPaging.mockUser, at }));
    await this.backend.reply(null);
  }
}
