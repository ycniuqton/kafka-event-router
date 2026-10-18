import { describe, expect, it } from 'vitest';
import { TestCasesService, type TestCaseDraft } from '../../../../api/contracts';
import {
  EventStatus,
  StepFactory,
  TestVerdict,
  TransformStepKind,
  type NewTestCase,
  type SampleEvent,
  type TestCase,
  type TestCasePatch,
  type TestCaseRunReport,
} from '../../../../domain';
import type { StudioEventRun } from '../preview/StudioRunResult';
import { TestCaseController, type TestCaseState } from './TestCaseController';

/** In-memory service whose runs resolve when the test releases them. */
class FakeTestCases extends TestCasesService {
  cases: TestCase[] = [];
  runs: { draft: TestCaseDraft; resolve(r: TestCaseRunReport): void }[] = [];
  /** Answer of single-case confirmation runs (pin); other runs wait for the test. */
  confirm: (caseId: string) => TestCaseRunReport = (caseId) => report(caseId, TestVerdict.Pass);

  list(): Promise<TestCase[]> {
    return Promise.resolve([...this.cases]);
  }
  create(routeId: string, c: NewTestCase): Promise<TestCase> {
    const created: TestCase = { ...c, id: `c${this.cases.length + 1}`, routeId, branchId: 'main', createdBy: 'me', createdAt: '', updatedAt: '', version: 1 };
    this.cases.push(created);
    return Promise.resolve(created);
  }
  update(c: TestCase, patch: TestCasePatch): Promise<TestCase> {
    return Promise.resolve({ ...c, ...patch, version: c.version + 1 });
  }
  remove(): Promise<void> {
    return Promise.resolve();
  }
  run(_routeId: string, draft: TestCaseDraft): Promise<TestCaseRunReport> {
    if (draft.caseIds) return Promise.resolve(this.confirm(draft.caseIds[0]));
    return new Promise((resolve) => this.runs.push({ draft, resolve }));
  }
}

const sample = { id: 3, key: '', headers: { h: '1' }, value: { a: 1 }, topic: 'orders', partition: 0, offset: 3, timestamp: '2026-10-14T00:00:00Z' } as unknown as SampleEvent;
const passedRun = { sampleId: 3, status: EventStatus.Passed, output: { key: '', headers: { h: '1' }, value: { b: 1 } } } as unknown as StudioEventRun;
const report = (caseId: string, result: TestVerdict): TestCaseRunReport => ({
  results: [{ caseId, name: 'x', branchId: 'main', result, actualOutcome: EventStatus.Passed, actualOutput: null, diff: [] }],
  passed: result === TestVerdict.Pass ? 1 : 0,
  failed: result === TestVerdict.Pass ? 0 : 1,
});
const flush = () => new Promise((r) => setTimeout(r, 0));

function setup() {
  const service = new FakeTestCases();
  const states: TestCaseState[] = [];
  const controller = new TestCaseController(service, 'r1', (s) => states.push(s));
  return { service, controller, last: () => states[states.length - 1] };
}

describe('TestCaseController', () => {
  it('pins the current event with its output and the input in engine shape', () => {
    const pin = TestCaseController.pinOf(sample, passedRun, '#3');
    expect(pin.inputEvent).toEqual({ key: null, value: { a: 1 }, headers: [{ key: 'h', value: '1' }], topic: 'orders', timestamp: '2026-10-14T00:00:00Z' });
    expect(pin.expectedOutcome).toBe(EventStatus.Passed);
    expect(pin.expectedOutput).toEqual({ key: null, value: { b: 1 }, headers: { h: '1' } });
    const dropped = TestCaseController.expectation({ ...passedRun, status: EventStatus.Dropped, output: null });
    expect(dropped).toEqual({ expectedOutcome: EventStatus.Dropped, expectedOutput: null });
  });

  it('re-runs after each preview, sends only complete steps and keeps the newest verdicts', async () => {
    const { service, controller, last } = setup();
    await controller.load();
    await controller.pin(TestCaseController.pinOf(sample, passedRun, '#3'));
    const complete = StepFactory.create(TransformStepKind.Rename, { field: 'a', argument: 'b' });
    const incomplete = StepFactory.create(TransformStepKind.Rename, { field: 'a', argument: '' });
    void controller.rerun([complete, incomplete]);
    void controller.rerun([complete]);
    expect(service.runs).toHaveLength(2);
    expect(service.runs[0].draft.steps).toEqual([complete]);
    service.runs[1].resolve(report('c1', TestVerdict.Pass));
    await flush();
    service.runs[0].resolve(report('c1', TestVerdict.Fail));
    await flush();
    expect(last().results.get('c1')?.result).toBe(TestVerdict.Pass);
    expect(last().running).toBe(false);
  });

  it('does not run without cases and re-runs the last steps after each write', async () => {
    const { service, controller } = setup();
    await controller.load();
    await controller.rerun([]);
    expect(service.runs).toHaveLength(0);
    const created = (await controller.pin(TestCaseController.pinOf(sample, passedRun, '#3'))) as TestCase;
    expect(service.runs).toHaveLength(1);
    await controller.relock(created, { expectedOutcome: EventStatus.Dropped, expectedOutput: null });
    expect(controller.snapshot.cases[0].expectedOutcome).toBe(EventStatus.Dropped);
    expect(service.runs).toHaveLength(2);
  });

  it('locks the engine result when the pinned (masked) output does not match it', async () => {
    const { service, controller } = setup();
    await controller.load();
    service.confirm = (caseId) => ({
      results: [{ caseId, name: 'x', branchId: 'main', result: TestVerdict.Fail, actualOutcome: EventStatus.Passed, actualOutput: { key: null, value: { b: 2 }, headers: {} }, diff: [] }],
      passed: 0,
      failed: 1,
    });
    const created = await controller.pin(TestCaseController.pinOf(sample, passedRun, '#3'));
    expect(created?.expectedOutput).toEqual({ key: null, value: { b: 2 }, headers: {} });
    expect(controller.snapshot.cases[0].version).toBe(2);
  });

  it('shows a failed write and forgets deleted cases', async () => {
    const { service, controller, last } = setup();
    await controller.load();
    service.create = () => Promise.reject(new Error('This route already has the maximum number of test cases (200). Delete one first.'));
    expect(await controller.pin(TestCaseController.pinOf(sample, passedRun, '#3'))).toBeNull();
    expect(last().error).toContain('maximum');
    service.cases = [];
    service.create = FakeTestCases.prototype.create.bind(service);
    const created = (await controller.pin(TestCaseController.pinOf(sample, passedRun, '#3'))) as TestCase;
    await controller.remove(created);
    expect(last().cases).toEqual([]);
  });
});
