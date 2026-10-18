import {
  EventStatus,
  JsonPatchDiffer,
  TestCaseDocuments,
  TestCaseLimits,
  TestVerdict,
  TransformRunner,
  type JsonValue,
  type NewTestCase,
  type TestCase,
  type TestCasePatch,
  type TestCaseResult,
  type TestCaseRunReport,
  type TransformRunResult,
} from '../../domain';
import { TestCasesService, type TestCaseDraft } from '../contracts';
import type { MockBackend } from './MockBackend';

/** Texts of the mock server's validation errors (same wording as the API). */
export const MockTestCaseText = {
  tooMany: 'This route already has the maximum number of test cases (200). Delete one first.',
  outputRequired: 'A passed outcome needs the expected output.',
  notFound: 'The requested resource does not exist. It may have been deleted.',
  outcome: (want: string, got: string) => `expected ${want}, got ${got}`,
  output: (n: number) => `output differs at ${n} place(s)`,
} as const;

/** Default branch of a pinned case. */
const MockBranch = 'main';

/** Mock test cases: kept per route in memory, runs use the in-browser transform runner (no filter). */
export class MockTestCasesService extends TestCasesService {
  private readonly backend: MockBackend;
  private readonly cases = new Map<string, TestCase[]>();
  private lastId = 0;

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  list(routeId: string): Promise<TestCase[]> {
    return this.backend.reply(this.of(routeId));
  }

  create(routeId: string, input: NewTestCase): Promise<TestCase> {
    const list = this.of(routeId);
    if (list.length >= TestCaseLimits.maxCases) return this.backend.fail(MockTestCaseText.tooMany);
    if (input.expectedOutcome === EventStatus.Passed && input.expectedOutput === null) return this.backend.fail(MockTestCaseText.outputRequired);
    this.lastId += 1;
    const now = this.backend.nowIso();
    const created: TestCase = { ...input, id: `tc-${this.lastId}`, routeId, branchId: input.branchId ?? MockBranch, createdBy: 'You', createdAt: now, updatedAt: now, version: 1 };
    this.cases.set(routeId, [...list, created]);
    return this.backend.reply(created);
  }

  update(testCase: TestCase, patch: TestCasePatch): Promise<TestCase> {
    const list = this.of(testCase.routeId);
    const current = list.find((c) => c.id === testCase.id);
    if (!current) return this.backend.fail(MockTestCaseText.notFound);
    const next: TestCase = { ...current, ...patch, version: current.version + 1, updatedAt: this.backend.nowIso() };
    if (next.expectedOutcome !== EventStatus.Passed) next.expectedOutput = null;
    this.cases.set(testCase.routeId, list.map((c) => (c.id === next.id ? next : c)));
    return this.backend.reply(next);
  }

  remove(testCase: TestCase): Promise<void> {
    this.cases.set(testCase.routeId, this.of(testCase.routeId).filter((c) => c.id !== testCase.id));
    return this.backend.reply(undefined);
  }

  run(routeId: string, draft: TestCaseDraft): Promise<TestCaseRunReport> {
    const results = this.of(routeId).filter((c) => !draft.caseIds || draft.caseIds.includes(c.id)).map((c) => MockCaseRun.result(c, TransformRunner.run(TestCaseDocuments.event(c.inputEvent), draft.steps)));
    const passed = results.filter((r) => r.result === TestVerdict.Pass).length;
    return this.backend.reply({ results, passed, failed: results.length - passed });
  }

  private of(routeId: string): TestCase[] {
    return this.cases.get(routeId) ?? [];
  }
}

/** Classifies a local run and compares it with the case's expectation. */
class MockCaseRun {
  static result(c: TestCase, run: TransformRunResult): TestCaseResult {
    const actualOutcome = run.error ? EventStatus.Error : run.dropped ? EventStatus.Dropped : EventStatus.Passed;
    const actualOutput = actualOutcome === EventStatus.Passed ? TestCaseDocuments.output(run.event) : null;
    const base = { caseId: c.id, name: c.name, branchId: c.branchId, actualOutcome, actualOutput };
    if (actualOutcome !== c.expectedOutcome) {
      return { ...base, result: TestVerdict.Fail, diff: [], error: run.error ?? MockTestCaseText.outcome(c.expectedOutcome, actualOutcome) };
    }
    const diff = actualOutcome === EventStatus.Passed ? JsonPatchDiffer.diff(c.expectedOutput, actualOutput as JsonValue) : [];
    const result = diff.length ? TestVerdict.Fail : TestVerdict.Pass;
    return { ...base, result, diff, error: diff.length ? MockTestCaseText.output(diff.length) : undefined };
  }
}
