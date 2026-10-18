import type { SampleEvent } from '../../../../domain';
import type { StudioRunResult } from '../preview/StudioRunResult';
import { StudioText } from './StudioText';

/** Failure of one sample: which event, which step, the message and (JS) the code location. */
export type SampleFailure = { sampleId: number; stepIndex: number; message: string; line: number | null; column: number | null };

/** Failures of every sample in the last preview run (design `errIds`, run summary and "Jump to #N"). */
export class SampleRunReport {
  readonly total: number;
  readonly failures: SampleFailure[];

  constructor(samples: SampleEvent[], run: StudioRunResult | null) {
    this.total = samples.length;
    this.failures = samples.flatMap((s) => {
      const error = run?.run(s.id)?.error;
      return error ? [{ sampleId: s.id, stepIndex: error.stepIndex ?? 0, message: error.message, line: error.line, column: error.column }] : [];
    });
  }

  get errorCount(): number {
    return this.failures.length;
  }

  get summary(): string {
    return StudioText.runSummary(this.total, this.errorCount);
  }

  get ranMessage(): string {
    return StudioText.ranOnSamples(this.total, this.errorCount);
  }

  isFailing(sampleId: number): boolean {
    return this.failures.some((f) => f.sampleId === sampleId);
  }

  /** First failing sample other than the current one, offered as a jump target. */
  jumpTarget(currentId: number | undefined): number | null {
    if (!this.failures.length || (currentId !== undefined && this.isFailing(currentId))) return null;
    return this.failures[0].sampleId;
  }

  /** Failure of a given step, preferring the current sample. */
  failureAtStep(stepIndex: number, currentId: number | undefined): SampleFailure | undefined {
    const atStep = this.failures.filter((f) => f.stepIndex === stepIndex);
    return atStep.find((f) => f.sampleId === currentId) ?? atStep[0];
  }
}
