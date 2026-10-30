import { LookupTestStepLabel, type ConnectionTestStep, type LookupTestResult, type TestStepState } from '../../../../domain';

/** Lookup test steps shown with the connection checklist rows (same states: passed / failed / skipped). */
export class LookupTestSteps {
  static rows(result: LookupTestResult): ConnectionTestStep[] {
    return result.steps.map((s) => ({
      label: LookupTestStepLabel[s.step],
      state: s.state as TestStepState,
      durationMs: s.durationMs,
      message: s.message || undefined,
      hint: s.hint || undefined,
    }));
  }
}
