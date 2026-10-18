import type { TestCasesService } from '../../../../api/contracts';
import { EventStatus, TestCaseDocuments, TestVerdict, type NewTestCase, type SampleEvent, type TestCase, type TestCaseResult, type TransformStep } from '../../../../domain';
import { StepCompleteness } from '../preview/StepCompleteness';
import type { StudioEventRun } from '../preview/StudioRunResult';

/** Saved cases of the route, the last verdict of each and the request state. */
export type TestCaseState = {
  cases: TestCase[];
  results: ReadonlyMap<string, TestCaseResult>;
  loading: boolean;
  running: boolean;
  error: string | null;
};

/** The locked expectation of a case. */
export type TestExpectation = Pick<TestCase, 'expectedOutcome' | 'expectedOutput'>;

/**
 * Saved test cases of one route in the Studio: loads them, pins events, and re-runs every case after
 * each successful preview (newest run wins; older responses are discarded).
 */
export class TestCaseController {
  static readonly initial: TestCaseState = { cases: [], results: new Map(), loading: true, running: false, error: null };

  private state: TestCaseState = TestCaseController.initial;
  private readonly service: TestCasesService;
  private readonly routeId: string;
  private readonly listener: (state: TestCaseState) => void;
  private steps: TransformStep[] | null = null;
  private runSeq = 0;
  private disposed = false;

  constructor(service: TestCasesService, routeId: string, listener: (state: TestCaseState) => void) {
    this.service = service;
    this.routeId = routeId;
    this.listener = listener;
  }

  /** Expectation of a sample from its Studio run: the outcome, and the output when it passed. */
  static expectation(run: StudioEventRun): TestExpectation {
    const passed = run.status === EventStatus.Passed;
    return { expectedOutcome: run.status, expectedOutput: passed ? TestCaseDocuments.output(run.output) : null };
  }

  /** A new case pinning `sample` with the outcome the Studio shows for it. */
  static pinOf(sample: SampleEvent, run: StudioEventRun, name: string): NewTestCase {
    return { name, inputEvent: TestCaseDocuments.input(sample), ...TestCaseController.expectation(run) };
  }

  async load(): Promise<void> {
    try {
      const cases = await this.service.list(this.routeId);
      this.set({ cases, loading: false, error: null });
      if (this.steps) await this.rerun(this.steps);
    } catch (e) {
      this.set({ loading: false, error: TestCaseController.message(e) });
    }
  }

  /** Runs every case against `steps` (incomplete steps left out, as in the preview). */
  async rerun(steps: TransformStep[]): Promise<void> {
    this.steps = steps;
    if (this.state.loading || !this.state.cases.length) return;
    const seq = ++this.runSeq;
    this.set({ running: true });
    try {
      const report = await this.service.run(this.routeId, { steps: TestCaseController.runnable(steps) });
      if (seq !== this.runSeq) return;
      this.set({ running: false, error: null, results: new Map(report.results.map((r) => [r.caseId, r])) });
    } catch (e) {
      if (seq === this.runSeq) this.set({ running: false, error: TestCaseController.message(e) });
    }
  }

  /**
   * Pins an event. The Studio may show masked values, so the expectation is confirmed against the engine:
   * when the new case does not pass on the current steps, the engine's actual result is locked instead.
   */
  async pin(input: NewTestCase): Promise<TestCase | null> {
    return this.mutate(async () => {
      const created = await this.service.create(this.routeId, input);
      const locked = await this.confirm(created);
      this.set({ cases: [...this.state.cases, locked] });
      return locked;
    });
  }

  async rename(testCase: TestCase, name: string): Promise<TestCase | null> {
    return this.mutate(() => this.replace(testCase, { name }));
  }

  /** Locks a new expectation (e.g. the current output, or the actual output of the last run). */
  async relock(testCase: TestCase, expectation: TestExpectation): Promise<TestCase | null> {
    return this.mutate(() => this.replace(testCase, expectation));
  }

  async remove(testCase: TestCase): Promise<void> {
    await this.mutate(async () => {
      await this.service.remove(testCase);
      const results = new Map(this.state.results);
      results.delete(testCase.id);
      this.set({ cases: this.state.cases.filter((c) => c.id !== testCase.id), results });
      return testCase;
    });
  }

  /** (Re)connects the listener; StrictMode remounts call it again after `dispose`. */
  activate(): void {
    this.disposed = false;
  }

  dispose(): void {
    this.disposed = true;
  }

  get snapshot(): TestCaseState {
    return this.state;
  }

  private async confirm(created: TestCase): Promise<TestCase> {
    const report = await this.service.run(this.routeId, { steps: TestCaseController.runnable(this.steps ?? []), caseIds: [created.id] });
    const result = report.results.find((r) => r.caseId === created.id);
    if (!result || result.result === TestVerdict.Pass || result.result === TestVerdict.Error) return created;
    return this.service.update(created, { expectedOutcome: result.actualOutcome, expectedOutput: result.actualOutput });
  }

  private async replace(testCase: TestCase, patch: Partial<TestCase>): Promise<TestCase> {
    const updated = await this.service.update(testCase, patch);
    this.set({ cases: this.state.cases.map((c) => (c.id === updated.id ? updated : c)) });
    return updated;
  }

  /** Runs a write; on success the cases are re-run against the last steps, on failure the error is shown. */
  private async mutate<T>(write: () => Promise<T>): Promise<T | null> {
    try {
      const out = await write();
      this.set({ error: null });
      if (this.steps) void this.rerun(this.steps);
      return out;
    } catch (e) {
      this.set({ error: TestCaseController.message(e) });
      return null;
    }
  }

  private set(patch: Partial<TestCaseState>): void {
    if (this.disposed) return;
    this.state = { ...this.state, ...patch };
    this.listener(this.state);
  }

  /** Steps the preview would run (incomplete ones are left out). */
  private static runnable(steps: TransformStep[]): TransformStep[] {
    return steps.filter((s) => StepCompleteness.reason(s) === null);
  }

  private static message(e: unknown): string {
    return e instanceof Error ? e.message : String(e);
  }
}
