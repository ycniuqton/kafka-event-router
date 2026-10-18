import type { PreviewService } from '../../../../api';
import { PreviewFailureKind, PreviewLimit, type PreviewFailure, type SampleEvent, type TransformStep } from '../../../../domain';
import { NoSampleSource, StudioRunPlan, type StudioSampleSource } from './StudioRunPlan';
import { StudioRunResult } from './StudioRunResult';

/** What the Studio shows of the preview: the last good result, whether a run is in flight, the last failure. */
export type StudioPreviewState = { result: StudioRunResult | null; pending: boolean; failure: PreviewFailure | null };

/** Timing and retry policy of Studio runs. */
export const StudioPreviewPolicy = { debounceMs: PreviewLimit.DebounceMs, maxRejectedSteps: 10 } as const;

/**
 * Runs the draft's steps on the samples through `services.preview` (same engine as workers): debounced,
 * newest edit wins (older runs are aborted and their responses discarded), and a step the preview rejects
 * is left out and flagged so the other steps keep previewing.
 */
export class StudioPreviewController {
  private readonly preview: PreviewService;
  private readonly onChange: (state: StudioPreviewState) => void;
  private state: StudioPreviewState = { result: null, pending: false, failure: null };
  private timer: ReturnType<typeof setTimeout> | null = null;
  private abort: AbortController | null = null;
  private latest = 0;
  private waiters: ((state: StudioPreviewState) => void)[] = [];

  constructor(preview: PreviewService, onChange: (state: StudioPreviewState) => void) {
    this.preview = preview;
    this.onChange = onChange;
  }

  /** Schedules a run of `steps` over `samples` copied from `source` (immediately when `now`). */
  schedule(steps: TransformStep[], samples: SampleEvent[], now = false, source: StudioSampleSource = NoSampleSource): void {
    this.cancel();
    const id = ++this.latest;
    if (!samples.length) {
      this.update({ result: null, pending: false, failure: null });
      return;
    }
    this.update({ ...this.state, pending: true });
    const start = () => void this.execute(id, steps, new StudioRunPlan(steps, samples, source));
    if (now) start();
    else this.timer = setTimeout(start, StudioPreviewPolicy.debounceMs);
  }

  /** Resolves when the newest scheduled run has settled (result or failure). */
  settled(): Promise<StudioPreviewState> {
    if (!this.state.pending) return Promise.resolve(this.state);
    return new Promise((resolve) => this.waiters.push(resolve));
  }

  dispose(): void {
    this.cancel();
    this.latest++;
  }

  private async execute(id: number, steps: TransformStep[], first: StudioRunPlan): Promise<void> {
    this.abort = new AbortController();
    let plan = first;
    for (let attempt = 0; attempt <= StudioPreviewPolicy.maxRejectedSteps; attempt++) {
      try {
        const result = await this.preview.run(plan.request, this.abort.signal);
        if (id === this.latest) this.update({ result: new StudioRunResult(plan, result), pending: false, failure: null });
        return;
      } catch (error) {
        const next = this.recover(id, steps, plan, this.preview.failure(error));
        if (!next) return;
        plan = next;
      }
    }
  }

  /** A plan without the rejected step, or null after reporting the failure (or when the run is stale). */
  private recover(id: number, steps: TransformStep[], plan: StudioRunPlan, failure: PreviewFailure): StudioRunPlan | null {
    if (id !== this.latest || failure.kind === PreviewFailureKind.Aborted) return null;
    const index = plan.draftIndex(StudioRunResult.runStepOf(failure.path));
    if (failure.kind === PreviewFailureKind.InvalidConfig && index !== null) return plan.without(steps, index, failure.fieldMessage ?? failure.message);
    this.update({ ...this.state, pending: false, failure });
    return null;
  }

  private cancel(): void {
    if (this.timer) clearTimeout(this.timer);
    this.timer = null;
    this.abort?.abort();
    this.abort = null;
  }

  private update(state: StudioPreviewState): void {
    this.state = state;
    this.onChange(state);
    if (state.pending) return;
    const waiters = this.waiters;
    this.waiters = [];
    waiters.forEach((resolve) => resolve(state));
  }
}
