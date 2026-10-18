import { ConnectionTestChecks, OptionalTestChecks, TestStepState, type ConnectionTestRun, type ConnectionTestStep } from '../../../domain';
import { TestLineType, TestStepDto, type StepEventDto, type TestLineDto } from './ConnectionDtos';

/** Server steps always shown, in checklist order; index i is labelled `ConnectionTestChecks[i]`. Optional steps follow. */
export const TestStepOrder: readonly string[] = [
  TestStepDto.BrokersReachable,
  TestStepDto.Authenticated,
  TestStepDto.ListTopics,
  TestStepDto.CanRead,
  TestStepDto.CanWrite,
];

/** Wire step states share the screen values; unknown values stay pending. */
const KnownStepStates: ReadonlySet<string> = new Set(Object.values(TestStepState));

/** Folds streamed step/result lines into `ConnectionTestRun` snapshots for live checklists. */
export class ConnectionTestAssembler {
  private readonly connectionId: string;
  private steps: ConnectionTestStep[];
  private finished = false;
  private ok = false;

  constructor(connectionId: string) {
    this.connectionId = connectionId;
    this.steps = ConnectionTestChecks.map((label) => ({ label, state: TestStepState.Pending }));
  }

  /** Applies one line and returns the new snapshot. */
  apply(line: TestLineDto): ConnectionTestRun {
    if (line.type === TestLineType.Result) {
      this.finished = true;
      this.ok = line.ok;
    } else {
      this.applyStep(line);
    }
    return this.snapshot();
  }

  snapshot(): ConnectionTestRun {
    return { connectionId: this.connectionId, steps: this.steps.map((s) => ({ ...s })), finished: this.finished, ok: this.ok };
  }

  private applyStep(line: StepEventDto): void {
    const index = this.indexOf(line.step);
    if (index < 0) return;
    const state = KnownStepStates.has(line.state) ? (line.state as TestStepState) : TestStepState.Pending;
    const reason = line.message ? [line.message, line.hint].filter(Boolean).join(' ') : undefined;
    const detail = { reason, message: line.message, hint: line.hint, durationMs: line.durationMs };
    const defined = Object.fromEntries(Object.entries(detail).filter(([, v]) => v !== undefined && v !== ''));
    this.steps = this.steps.map((s, i) => (i === index ? { label: s.label, state, ...defined } : s));
  }

  /** Row of a server step; optional steps (e.g. schema registry) get a row the first time they are reported. */
  private indexOf(step: string): number {
    const fixed = TestStepOrder.indexOf(step);
    if (fixed >= 0) return fixed;
    const label = OptionalTestChecks[step];
    if (!label) return -1;
    const existing = this.steps.findIndex((s) => s.label === label);
    if (existing >= 0) return existing;
    this.steps = [...this.steps, { label, state: TestStepState.Pending }];
    return this.steps.length - 1;
  }
}
