import { PreviewFailureKind, PreviewLimit, type PreviewFailure, type RunResult } from '../../domain';
import type { PreviewService } from '../../api';
import { PreviewPhase } from './PreviewState';
import type { PreviewStore } from './PreviewStore';
import type { SampleLoader } from './SampleLoader';

/** Timer port so tests can drive the debounce deterministically. */
export type RunTimers = { set: (fn: () => void, ms: number) => unknown; clear: (handle: unknown) => void };

/** Test run options: also write passed outputs to a temporary test topic (FR-22). */
export type TestRunOptions = { writeToTopic?: boolean; routeId?: string };

/** Failures retried once before they are shown: expired session (reload samples) and busy (wait). */
type RetryBudget = { reload: boolean; busy: boolean };

/**
 * Runs draft configs on the session: debounced auto-runs, immediate runs (Ctrl+Enter), stale runs aborted,
 * an expired session reloaded silently and a busy preview service retried once.
 */
export class PreviewRunController {
  private readonly store: PreviewStore;
  private readonly service: PreviewService;
  private readonly loader: SampleLoader;
  private readonly timers: RunTimers;
  private timer: unknown = null;
  private inFlight: AbortController | null = null;

  constructor(store: PreviewStore, service: PreviewService, loader: SampleLoader, timers?: RunTimers) {
    this.store = store;
    this.service = service;
    this.loader = loader;
    this.timers = timers ?? { set: (fn, ms) => setTimeout(fn, ms), clear: (h) => clearTimeout(h as ReturnType<typeof setTimeout>) };
  }

  /** Debounced run (draft edits). */
  schedule(config: unknown): void {
    this.cancelTimer();
    this.timer = this.timers.set(() => void this.runNow(config), PreviewLimit.DebounceMs);
  }

  /** Runs now, cancelling a pending or in-flight run; resolves null when superseded or nothing is loaded. */
  async runNow(config: unknown): Promise<RunResult | null> {
    this.cancelTimer();
    this.inFlight?.abort();
    const abort = new AbortController();
    this.inFlight = abort;
    this.store.setState({ runPhase: PreviewPhase.Loading });
    const result = await this.attempt(config, abort, { reload: true, busy: true });
    if (this.inFlight === abort) this.inFlight = null;
    return result;
  }

  /** The Test button: full route on the loaded samples with mandatory output validation (optionally into a test topic). */
  async test(config: unknown, options: TestRunOptions = {}): Promise<RunResult | null> {
    const session = this.store.getState().session;
    if (!session) return null;
    this.store.setState({ testPhase: PreviewPhase.Loading, testResult: options.writeToTopic ? this.store.getState().testResult : null });
    try {
      const testResult = await this.service.testRun({ sessionId: session.id, config, ...options });
      this.store.setState({ testResult, testPhase: PreviewPhase.Ready });
      return testResult;
    } catch (error) {
      this.store.setState({ testPhase: PreviewPhase.Failed, runFailure: this.service.failure(error) });
      return null;
    }
  }

  dispose(): void {
    this.cancelTimer();
    this.inFlight?.abort();
    this.inFlight = null;
  }

  private async attempt(config: unknown, abort: AbortController, retry: RetryBudget): Promise<RunResult | null> {
    const session = this.store.getState().session;
    if (!session) {
      this.store.setState({ runPhase: PreviewPhase.Idle });
      return null;
    }
    try {
      const result = await this.service.run({ sessionId: session.id, config }, abort.signal);
      if (abort.signal.aborted) return null;
      this.store.setState({ result, runPhase: PreviewPhase.Ready, runFailure: null });
      return result;
    } catch (error) {
      return this.recover(config, abort, retry, this.service.failure(error));
    }
  }

  private async recover(config: unknown, abort: AbortController, retry: RetryBudget, failure: PreviewFailure): Promise<RunResult | null> {
    if (abort.signal.aborted || failure.kind === PreviewFailureKind.Aborted) return null;
    if (failure.kind === PreviewFailureKind.SessionExpired && retry.reload) {
      const reloaded = await this.loader.reload();
      return reloaded && !abort.signal.aborted ? this.attempt(config, abort, { ...retry, reload: false }) : null;
    }
    if (failure.kind === PreviewFailureKind.Busy && retry.busy) {
      await new Promise((resolve) => this.timers.set(() => resolve(null), PreviewLimit.BusyRetryMs));
      return abort.signal.aborted ? null : this.attempt(config, abort, { ...retry, busy: false });
    }
    this.store.setState({ runPhase: PreviewPhase.Failed, runFailure: failure });
    return null;
  }

  private cancelTimer(): void {
    if (this.timer !== null) this.timers.clear(this.timer);
    this.timer = null;
  }
}
