import { SampleMode, PreviewLimit, type LoadSamplesRequest, type RunResult, type SampleView, type ValueFormat } from '../../domain';
import type { PreviewService } from '../../api';
import { PreviewRunController, type RunTimers, type TestRunOptions } from './PreviewRunController';
import type { PreviewStore } from './PreviewStore';
import { SampleLoader } from './SampleLoader';

/** Where latest / time-range samples are read from; `demo` = the demo sandbox (synthetic samples, LP-204). */
export type SampleSourceRef = { connectionId: string; topics: string[]; demo?: boolean };

/** Every change of one sample panel: loads, uploads, search, selection, view, runs and test runs. */
export class PreviewActions {
  readonly store: PreviewStore;
  private readonly loader: SampleLoader;
  private readonly runs: PreviewRunController;

  constructor(store: PreviewStore, service: PreviewService, timers?: RunTimers) {
    this.store = store;
    this.loader = new SampleLoader(store, service);
    this.runs = new PreviewRunController(store, service, this.loader, timers);
  }

  loadLatest(source: SampleSourceRef, count: number = PreviewLimit.DefaultCount): Promise<boolean> {
    return this.load(PreviewActions.latest(source, count));
  }

  private static latest(source: SampleSourceRef, count: number): LoadSamplesRequest {
    const mode = source.demo ? SampleMode.Demo : SampleMode.Latest;
    return { connectionId: source.connectionId, topics: source.topics, mode, count };
  }

  /** Loads the latest samples unless the same source is already loaded or loading. */
  ensureLatest(source: SampleSourceRef, count: number = PreviewLimit.DefaultCount): Promise<boolean> {
    const request = PreviewActions.latest(source, count);
    return this.loader.isCurrent(request) ? Promise.resolve(true) : this.load(request);
  }

  loadTimeRange(source: SampleSourceRef, from: string, to: string): Promise<boolean> {
    return this.load({ connectionId: source.connectionId, topics: source.topics, mode: SampleMode.TimeRange, from, to });
  }

  /** Uploaded JSON array / NDJSON text; the source connection is optional (decision 5). */
  upload(text: string, source?: SampleSourceRef): Promise<boolean> {
    const connectionId = source?.connectionId || undefined;
    return this.load({ connectionId, topics: source?.topics.length ? source.topics : undefined, mode: SampleMode.Upload, upload: text });
  }

  attach(sessionId: string): Promise<boolean> {
    return this.loader.attach(sessionId);
  }

  reload(): Promise<boolean> {
    return this.loader.reload();
  }

  search(query: string): Promise<void> {
    return this.loader.search(query);
  }

  select(seq: number | null): void {
    this.store.setState({ selectedSeq: seq });
  }

  setView(view: SampleView): void {
    this.store.setState({ view });
  }

  /** Shows one branch of a multi-branch route (null = first branch). */
  setBranch(branch: string | null): void {
    this.store.setState({ branch });
  }

  /** Overrides the session's value format (re-decodes the stored raw bytes); the next run uses the new view. */
  reformat(format: ValueFormat): Promise<boolean> {
    return this.loader.reformat(format);
  }

  /** Debounced run after a draft edit. */
  scheduleRun(config: unknown): void {
    this.runs.schedule(config);
  }

  runNow(config: unknown): Promise<RunResult | null> {
    return this.runs.runNow(config);
  }

  test(config: unknown, options?: TestRunOptions): Promise<RunResult | null> {
    return this.runs.test(config, options);
  }

  /** Re-runs the test and writes its passed outputs to a router.test.* topic on the destination connection. */
  writeTestTopic(config: unknown, routeId?: string): Promise<RunResult | null> {
    return this.runs.test(config, { writeToTopic: true, routeId });
  }

  clearTest(): void {
    this.store.setState({ testResult: null });
  }

  /** Stops runs and closes the session (panel unmount). */
  dispose(): void {
    this.runs.dispose();
    this.loader.cancel();
    this.loader.closeCurrent();
  }

  private async load(request: LoadSamplesRequest): Promise<boolean> {
    return this.loader.load(request);
  }
}
