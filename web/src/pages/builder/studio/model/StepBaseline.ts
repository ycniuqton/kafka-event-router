import type { TransformStep } from '../../../../domain';

/** The steps as last saved, to mark cards whose step changed since (FR-02 unsaved-change dot). */
export class StepBaseline {
  private readonly saved: ReadonlyMap<number, string>;

  constructor(steps: TransformStep[]) {
    this.saved = new Map(steps.map((s) => [s.id, StepBaseline.key(s)]));
  }

  isDirty(step: TransformStep): boolean {
    return this.saved.get(step.id) !== StepBaseline.key(step);
  }

  private static key(step: TransformStep): string {
    const { explained: _explained, ...config } = step;
    return JSON.stringify(config);
  }
}
