import {
  CauseFingerprint,
  DeadLetterClasses,
  DeadLetterGroups,
  DryRunOutcome,
  ReplayRate,
  DeadLetterReplayStatus,
  StructuralDiff,
  type DeadLetterEdit,
  type DeadLetterGroupPage,
  type DeadLetterJobOptions,
  type DryRunReport,
  type DeadLetterReplayOptions,
  type DeadLetterReplayResult,
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
import { MockDryRunner, MockFourEyes } from './MockDeadLetterWorkflow';

/** Export file naming and media types shared with the REST export. */
export const DeadLetterExportFormat = {
  mimeType: 'application/json',
  ndjsonMimeType: 'application/x-ndjson',
  prefix: 'dead-letter-',
  indent: 2,
  lineBreak: '\n',
} as const;

/** Mock paging: page size and the cursor = index of the next row; jobs process `jobStep` events per poll. */
export const MockDeadLetterPaging = { pageSize: 50, syncCap: DeadLetterReports.syncCap, jobStep: 2, mockUser: 'u-me', dryRunSample: 20 } as const;

/** A replay another user asked for (the mock's approvals inbox is never empty for the 4-eyes demo). */
export const MockPendingReplay = { id: 'job-approval-1', routeId: 'r5', ids: ['d4'], requestedBy: 'u-ana', note: 'Amounts arrive as strings from the new checkout; replay after the fix.' } as const;

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
    if (f.classes?.length && !f.classes.includes(DeadLetterClasses.of(d))) return false;
    if (f.fingerprint && CauseFingerprint.ofRow(d) !== f.fingerprint) return false;
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
  private readonly fourEyes: MockFourEyes;
  private readonly dryRunner: MockDryRunner;

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
    this.fourEyes = new MockFourEyes(backend);
    this.dryRunner = new MockDryRunner(backend);
    this.seedPendingReplay();
  }

  page(filter: DeadLetterFilter, cursor?: string | null): Promise<DeadLetterPage> {
    const matching = this.matching(filter);
    const start = Number(cursor ?? 0);
    const end = start + MockDeadLetterPaging.pageSize;
    const items = matching.slice(start, end).map((d) => ({ ...this.classified(d), routeName: this.routeName(d.routeId), resolution: this.resolutions.get(d.id) ?? null }));
    return this.backend.reply({ items, nextCursor: end < matching.length ? String(end) : null, truncated: false });
  }

  summary(routeId?: string): Promise<DeadLetterSummary> {
    const all = this.matching({ routeId, kinds: [] });
    const routeIds = [...new Set(all.map((d) => d.routeId))];
    const routes = routeIds.map((id) => {
      const own = all.filter((d) => d.routeId === id);
      return { routeId: id, routeName: this.routeName(id), unresolved: own.length, byKind: DeadLetterSummaries.kinds(own) };
    });
    const byKind = DeadLetterSummaries.kinds(all);
    return this.backend.reply({ unresolved: all.length, approximate: false, byKind, routes, byClass: DeadLetterClasses.counts(byKind) });
  }

  /** Detail; events of routes reading a `sensitive` connection come back masked. */
  detail(deadLetter: DeadLetter): Promise<DeadLetterDetail> {
    const sample = SAMPLES.find((e) => e.id === deadLetter.sampleId);
    if (!sample) return this.backend.reply({ stack: deadLetter.stack, original: null });
    const masked = this.sensitive(deadLetter.routeId);
    const show = (v: unknown) => (masked ? MockValueMasker.mask(v) : v);
    return this.backend.reply({ stack: deadLetter.stack, original: { key: show(sample.key), value: show(sample.value), headers: [], rawBytes: false, masked, keyEncoding: 'json', valueEncoding: 'json' } });
  }

  /** Events of routes writing to prod are not sent: they come back as `approvalRequired` (strict 4-eyes). */
  async retry(ids: string[]): Promise<DeadLetterActionReport> {
    const gated = this.backend.db.deadLetters.filter((d) => ids.includes(d.id) && this.fourEyes.requiresApproval(d.routeId));
    const free = ids.filter((id) => !gated.some((d) => d.id === id));
    await this.resolve(free, DeadLetterResolutionState.Retried);
    const rows = this.backend.db.deadLetters.filter((d) => free.includes(d.id));
    const base = DeadLetterReports.allResolved(free, true);
    const report = DeadLetterDelivery.exactlyOnce(rows) ? { ...base, deliveredAtLeastOnce: true } : base;
    const routes = [...new Set(gated.map((d) => d.routeId))];
    return routes.length ? { ...report, approvalRequired: routes.map((routeId) => ({ routeId, ids: gated.filter((d) => d.routeId === routeId).map((d) => d.id) })) } : report;
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

  startJob(routeId: string, action: DeadLetterJobAction, filter: DeadLetterFilter, options: DeadLetterJobOptions = {}): Promise<DeadLetterJob> {
    const pending = options.ids ?? this.matching({ ...filter, routeId, includeResolved: false }).map((d) => d.id);
    const gated = action === DeadLetterJobAction.Retry && this.fourEyes.requiresApproval(routeId);
    const job = this.newJob(routeId, action, pending.length, options, gated);
    this.jobs.set(job.id, { job, pending: [...pending] });
    return this.backend.reply(job);
  }

  private newJob(routeId: string, action: DeadLetterJobAction, total: number, options: DeadLetterJobOptions, gated: boolean): DeadLetterJob {
    return {
      id: `job-${this.jobs.size + 1}`,
      routeId,
      routeName: this.routeName(routeId),
      action,
      status: gated ? DeadLetterJobStatus.PendingApproval : DeadLetterJobStatus.Queued,
      total,
      processed: 0,
      counts: {},
      error: null,
      createdAt: this.backend.clock.now().toISOString(),
      finishedAt: null,
      ratePerSecond: options.ratePerSecond ?? ReplayRate.default,
      note: options.note ?? '',
      idsCount: options.ids?.length ?? 0,
      requestedBy: MockDeadLetterPaging.mockUser,
      requestedByMe: true,
      approval: { required: gated },
    };
  }

  /** Each poll processes the next `jobStep` events (the mock's notion of time). */
  async job(jobId: string): Promise<DeadLetterJob> {
    const entry = this.jobs.get(jobId);
    if (!entry) return this.backend.fail(`Job ${jobId} does not exist.`);
    if (!DeadLetterJobs.done(entry.job) && !DeadLetterJobs.awaitingApproval(entry.job)) await this.advance(entry);
    return this.backend.reply(entry.job);
  }

  cancelJob(jobId: string): Promise<DeadLetterJob> {
    const entry = this.jobs.get(jobId);
    if (!entry) return this.backend.fail(`Job ${jobId} does not exist.`);
    if (!DeadLetterJobs.done(entry.job)) entry.job = { ...entry.job, status: DeadLetterJobStatus.Cancelled, finishedAt: this.backend.clock.now().toISOString() };
    return this.backend.reply(entry.job);
  }

  groups(filter: DeadLetterFilter): Promise<DeadLetterGroupPage> {
    const rows = this.matching({ ...filter, fingerprint: undefined }).map((d) => this.classified(d));
    return this.backend.reply(DeadLetterGroups.build(rows, this.backend.clock.now(), (id) => this.routeName(id)));
  }

  async replay(deadLetter: DeadLetter, edit: DeadLetterEdit | null, options: DeadLetterReplayOptions): Promise<DeadLetterReplayResult> {
    const original = (await this.detail(deadLetter)).original;
    const before = { key: original?.key ?? null, value: original?.value ?? null, headers: original?.headers ?? [] };
    const diff = edit ? StructuralDiff.of(before, edit) : [];
    const masked = !!original?.masked;
    if (options.dryRun) {
      const preview = this.dryRunner.run(deadLetter, original, edit);
      return this.backend.reply({ status: DeadLetterReplayStatus.Preview, diff, masked, activeVersionNo: this.activeVersion(deadLetter.routeId), preview });
    }
    if (this.fourEyes.requiresApproval(deadLetter.routeId)) return this.requestReplay(deadLetter, edit, options, diff, masked);
    await this.resolve([deadLetter.id], DeadLetterResolutionState.Retried);
    return { status: DeadLetterReplayStatus.Done, diff, masked, report: DeadLetterReports.allResolved([deadLetter.id], true) };
  }

  private async requestReplay(d: DeadLetter, edit: DeadLetterEdit | null, options: DeadLetterReplayOptions, diff: DeadLetterReplayResult['diff'], masked: boolean): Promise<DeadLetterReplayResult> {
    const started = await this.startJob(d.routeId, DeadLetterJobAction.Retry, { kinds: [] }, { ids: [d.id], note: options.note });
    const entry = this.jobs.get(started.id);
    const job = { ...started, edited: !!edit };
    if (entry) entry.job = job;
    return { status: DeadLetterReplayStatus.PendingApproval, diff, masked, job };
  }

  async dryRun(routeId: string, selection: DeadLetterSelection): Promise<DryRunReport> {
    const rows = 'ids' in selection ? this.backend.db.deadLetters.filter((d) => selection.ids.includes(d.id)) : this.matching({ ...selection.filter, routeId, includeResolved: false });
    const sample = rows.slice(0, MockDeadLetterPaging.dryRunSample);
    const results = await Promise.all(sample.map(async (d) => this.dryRunner.run(d, (await this.detail(d)).original, null)));
    const counts: Partial<Record<DryRunOutcome, number>> = {};
    results.forEach((r) => (counts[r.outcome] = (counts[r.outcome] ?? 0) + 1));
    return this.backend.reply({ activeVersionNo: this.activeVersion(routeId), sampled: results.length, results, counts });
  }

  approveJob(jobId: string, note: string): Promise<DeadLetterJob> {
    return this.decide(jobId, note, DeadLetterJobStatus.Queued);
  }

  rejectJob(jobId: string, note: string): Promise<DeadLetterJob> {
    return this.decide(jobId, note, DeadLetterJobStatus.Rejected);
  }

  pendingApprovals(): Promise<DeadLetterJob[]> {
    return this.backend.reply([...this.jobs.values()].map((e) => e.job).filter((j) => DeadLetterJobs.awaitingApproval(j)));
  }

  /** Strict 4-eyes: the requester never decides; a decided job leaves the pending state. */
  private decide(jobId: string, note: string, status: DeadLetterJobStatus): Promise<DeadLetterJob> {
    const entry = this.jobs.get(jobId);
    if (!entry || !DeadLetterJobs.awaitingApproval(entry.job)) return this.backend.fail('This replay is not waiting for approval any more.');
    if (entry.job.requestedByMe) return this.backend.fail("You can't approve your own replay: another approver has to.");
    const at = this.backend.nowIso();
    const finishedAt = status === DeadLetterJobStatus.Rejected ? at : null;
    entry.job = { ...entry.job, status, finishedAt, approval: { required: true, decidedBy: MockDeadLetterPaging.mockUser, decidedAt: at, decisionNote: note } };
    return this.backend.reply(entry.job);
  }

  private seedPendingReplay(): void {
    const p = MockPendingReplay;
    const base = this.newJob(p.routeId, DeadLetterJobAction.Retry, p.ids.length, { ids: [...p.ids], note: p.note }, true);
    const job = { ...base, id: p.id, requestedBy: p.requestedBy, requestedByMe: false };
    this.jobs.set(job.id, { job, pending: [...p.ids] });
  }

  private classified(d: DeadLetter): DeadLetter {
    return { ...d, errorClass: DeadLetterClasses.of(d), fingerprint: CauseFingerprint.ofRow(d) };
  }

  private activeVersion(routeId: string): number {
    return this.backend.db.routes.find((r) => r.id === routeId)?.version ?? 1;
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
