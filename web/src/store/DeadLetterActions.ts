import type { Services } from '../api';
import {
  DeadLetterFileFormat,
  DeadLetterJobAction,
  DeadLetterJobStatus,
  DeadLetterJobs,
  DeadLetterSkipReason,
  IntentType,
  RouteDetailTab,
  Tone,
  type Alert,
  type DeadLetter,
  type DeadLetterActionReport,
  type DeadLetterDetail,
  type DeadLetterExportFile,
  type DeadLetterFilter,
  type DeadLetterJob,
  type DeadLetterPage,
  type DeadLetterSelection,
  type DeadLetterStudioLink,
} from '../domain';
import type { AppStore } from './AppStore';
import type { ToastController } from './ToastController';

/** Toast texts of dead-letter actions (design wording). */
export class DeadLetterToasts {
  static retried(r: DeadLetterActionReport): string {
    const done = r.resolved.length;
    const head = done === 1 ? 'Retried 1 event · sent' : `Retried ${done} events`;
    const failed = r.failed.length ? ` · ${r.failed.length} failed again: ${r.failed[0].message}` : '';
    const locked = r.skipped.some((s) => s.reason === DeadLetterSkipReason.Locked) ? ' · some events are being retried by someone else' : '';
    const remaining = r.remaining > 0 ? ` · ${r.remaining} more remain, retry again` : '';
    const atLeastOnce = r.deliveredAtLeastOnce ? ' · delivered at-least-once' : '';
    return head + failed + locked + remaining + atLeastOnce;
  }

  static discarded(r: DeadLetterActionReport): string {
    return r.resolved.length === 1 ? 'Event discarded' : `Discarded ${r.resolved.length} events`;
  }

  static restored(r: DeadLetterActionReport): string {
    const skipped = r.skipped.length ? ` · ${r.skipped.length} not discarded` : '';
    return (r.resolved.length === 1 ? 'Event restored' : `Restored ${r.resolved.length} events`) + skipped;
  }

  static jobStarted(job: DeadLetterJob): string {
    return `${job.action === DeadLetterJobAction.Retry ? 'Retry' : 'Discard'} of ${job.total || 'all'} matching events started in the background`;
  }

  static jobEnded(job: DeadLetterJob): string {
    const verb = job.action === DeadLetterJobAction.Retry ? 'Background retry' : 'Background discard';
    const outcome = job.status === DeadLetterJobStatus.Succeeded ? 'finished' : job.status.replace('_', ' ');
    const failed = job.counts.failed ? ` · ${job.counts.failed} failed again` : '';
    return `${verb} ${outcome}: ${job.processed} events${failed}`;
  }

  static failure(error: unknown): string {
    return error instanceof Error ? error.message : String(error);
  }
}

/** Browse, retry, discard, export and Open-in-Studio of dead-lettered events; keeps the badge summary fresh. */
export class DeadLetterActions {
  private readonly store: AppStore;
  private readonly services: Services;
  private readonly toast: ToastController;

  constructor(store: AppStore, services: Services, toast: ToastController) {
    this.store = store;
    this.services = services;
    this.toast = toast;
  }

  page(filter: DeadLetterFilter, cursor?: string | null): Promise<DeadLetterPage> {
    return this.services.deadLetters.page(filter, cursor);
  }

  /** Reloads the global unresolved counts (nav badge, tab label, dashboard card, chips); failures keep the last value. */
  async refreshSummary(): Promise<void> {
    try {
      this.store.setState({ deadLetterSummary: await this.services.deadLetters.summary() });
    } catch (error) {
      console.warn(DeadLetterToasts.failure(error));
    }
  }

  /** Retries events; resolved ones leave the queue, failed ones stay (their error is reported). */
  retry(ids: string[]): Promise<DeadLetterActionReport | null> {
    return this.act(() => this.services.deadLetters.retry(ids), DeadLetterToasts.retried);
  }

  /** "Retry all" of one route: every event matching the filter, up to the sync cap. */
  retryMatching(routeId: string, filter: DeadLetterFilter): Promise<DeadLetterActionReport | null> {
    return this.act(() => this.services.deadLetters.retryMatching(routeId, filter), DeadLetterToasts.retried);
  }

  discard(ids: string[]): Promise<DeadLetterActionReport | null> {
    return this.act(() => this.services.deadLetters.discard(ids), DeadLetterToasts.discarded);
  }

  /** Un-discards events (P2); the browser reloads to show them again. */
  restore(ids: string[]): Promise<DeadLetterActionReport | null> {
    return this.act(() => this.services.deadLetters.restore(ids), DeadLetterToasts.restored);
  }

  /** Starts a background job over every matching event of a route and follows it until it ends. */
  async startJob(routeId: string, action: DeadLetterJobAction, filter: DeadLetterFilter): Promise<DeadLetterJob | null> {
    try {
      const job = await this.services.deadLetters.startJob(routeId, action, filter);
      this.track(job);
      this.toast.show(DeadLetterToasts.jobStarted(job));
      this.follow(job.id);
      return job;
    } catch (error) {
      this.toast.show(DeadLetterToasts.failure(error));
      return null;
    }
  }

  async cancelJob(jobId: string): Promise<void> {
    try {
      this.track(await this.services.deadLetters.cancelJob(jobId));
    } catch (error) {
      this.toast.show(DeadLetterToasts.failure(error));
    }
  }

  /** Forgets a finished job (its progress card is dismissed). */
  dismissJob(jobId: string): void {
    this.store.setState({ deadLetterJobs: this.store.getState().deadLetterJobs.filter((j) => j.id !== jobId) });
  }

  /** Polls a job until it ends, then notifies (toast + alerts menu) and refreshes the counts. */
  private follow(jobId: string): void {
    setTimeout(() => {
      this.services.deadLetters.job(jobId).then(
        (job) => {
          this.track(job);
          if (DeadLetterJobs.done(job)) this.ended(job);
          else this.follow(jobId);
        },
        () => this.follow(jobId),
      );
    }, DeadLetterJobs.pollMs);
  }

  private ended(job: DeadLetterJob): void {
    const message = DeadLetterToasts.jobEnded(job);
    this.toast.show(message);
    const tone = job.status === DeadLetterJobStatus.Succeeded && !job.counts.failed ? Tone.Ok : Tone.Warn;
    const alert: Alert = { id: `dlq-job-${job.id}`, tone, title: message, detail: job.routeName, intent: { type: IntentType.RouteDetail, routeId: job.routeId, tab: RouteDetailTab.Dead } };
    this.store.setState({ alerts: [alert, ...this.store.getState().alerts.filter((a) => a.id !== alert.id)] });
    void this.refreshSummary();
  }

  private track(job: DeadLetterJob): void {
    const others = this.store.getState().deadLetterJobs.filter((j) => j.id !== job.id);
    this.store.setState({ deadLetterJobs: [job, ...others] });
  }

  detail(deadLetter: DeadLetter): Promise<DeadLetterDetail> {
    return this.services.deadLetters.detail(deadLetter);
  }

  async exportJson(routeId: string, selection: DeadLetterSelection, format: DeadLetterFileFormat = DeadLetterFileFormat.Json): Promise<DeadLetterExportFile | null> {
    try {
      return await this.services.deadLetters.exportJson(routeId, selection, format);
    } catch (error) {
      this.toast.show(DeadLetterToasts.failure(error));
      return null;
    }
  }

  async openInStudio(deadLetter: DeadLetter): Promise<DeadLetterStudioLink | null> {
    try {
      return await this.services.deadLetters.openInStudio(deadLetter);
    } catch (error) {
      this.toast.show(DeadLetterToasts.failure(error));
      return null;
    }
  }

  private async act(run: () => Promise<DeadLetterActionReport>, toast: (r: DeadLetterActionReport) => string): Promise<DeadLetterActionReport | null> {
    try {
      const report = await run();
      this.toast.show(toast(report));
      void this.refreshSummary();
      return report;
    } catch (error) {
      this.toast.show(DeadLetterToasts.failure(error));
      return null;
    }
  }
}
