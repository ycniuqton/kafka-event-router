import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { PreviewService } from '../../../../api/contracts';
import {
  EventStatus,
  PreviewFailureKind,
  StepFactory,
  TransformStepKind,
  type PreviewFailure,
  type RunRequest,
  type RunResult,
  type SampleEvent,
} from '../../../../domain';
import { PatchReplayer } from './PatchReplayer';
import { StudioPreviewController, type StudioPreviewState } from './StudioPreviewController';
import { StudioRunDocument, StudioRunPlan } from './StudioRunPlan';
import { StudioRunResult } from './StudioRunResult';

/** Sample events of the tests. */
class TestSamples {
  static of(id: number, value: Record<string, unknown>): SampleEvent {
    return { id, topic: 'orders', partition: 0, offset: id, timestamp: '2026-10-14T00:00:00Z', key: `k${id}`, headers: {}, value } as unknown as SampleEvent;
  }
}

const steps = [
  StepFactory.create(TransformStepKind.Mask, { field: 'email' }),
  StepFactory.create(TransformStepKind.Rename, { field: 'a', argument: '' }),
  StepFactory.create(TransformStepKind.Js, { code: 'function transform(e){ return e.value.q.z; }' }),
];
const samples = [TestSamples.of(7, { email: 'a@b.c', a: 1 }), TestSamples.of(9, { email: 'x@y.z', a: 2 })];

/** The preview's answer for `steps` minus the incomplete rename: event 7 fails in JS at 1:31, event 9 is dropped. */
const runResult: RunResult = {
  summary: { loaded: 2, processed: 2, passed: 0, filtered: 0, errors: 1, dropped: 1, incomplete: false, durationMs: 4 },
  issues: [],
  results: [
    {
      seq: 0,
      status: EventStatus.Error,
      error: { stepIndex: 1, errorType: 'step', message: "TypeError: cannot read property 'z' of undefined (1:31)" },
      branches: [{ branch: 'main', status: EventStatus.Error, filter: [], error: null, outputs: [], steps: [
        { index: 0, id: 's0', kind: 'mask', patch: [{ op: 'replace', path: '/value/email', value: '*@b.c' }], durationUs: 1, logs: [], error: null },
        { index: 1, id: 's1', kind: 'js', patch: [{ op: 'remove', path: '/0' }], durationUs: 1, logs: ['hi'], error: 'TypeError (1:31)' },
      ] }],
    },
    {
      seq: 1,
      status: EventStatus.Dropped,
      error: null,
      branches: [{ branch: 'main', status: EventStatus.Dropped, filter: [], error: null, outputs: [], steps: [
        { index: 0, id: 's0', kind: 'mask', patch: [{ op: 'replace', path: '/value/email', value: '*@y.z' }], durationUs: 1, logs: [], error: null },
        { index: 1, id: 's1', kind: 'js', patch: [{ op: 'remove', path: '/0' }], durationUs: 1, logs: [], error: null },
      ] }],
    },
  ],
};

describe('PatchReplayer', () => {
  it('applies event patches, then list patches once the event is dropped or fanned out', () => {
    const lists = PatchReplayer.replay({ key: 'k', value: { a: 1 }, headers: [] }, [
      [{ op: 'add', path: '/value/b', value: 2 }],
      [],
      [{ op: 'add', path: '/1', value: { key: 'k2', value: {}, headers: [] } }],
      [{ op: 'remove', path: '/1' }, { op: 'remove', path: '/0' }],
    ]);
    expect(lists[0]).toEqual([{ key: 'k', value: { a: 1, b: 2 }, headers: [] }]);
    expect(lists[1]).toBe(lists[0]);
    expect(lists[2]).toHaveLength(2);
    expect(lists[3]).toEqual([]);
  });

  it('ends with an empty list when a patch no longer applies', () => {
    expect(PatchReplayer.step([{ value: 1 }], [{ op: 'replace', path: '/value/x/y', value: 1 }])).toEqual([]);
  });
});

describe('StudioRunPlan', () => {
  it('skips incomplete steps, maps run step indexes back and sends samples inline', () => {
    const plan = new StudioRunPlan(steps, samples);
    expect(plan.skipped.get(1)).toBeTruthy();
    expect(plan.stepIndexes).toEqual([0, 2]);
    expect(plan.draftIndex(1)).toBe(2);
    const config = plan.request.config as { branches: { transform: { steps: { kind: string }[] } }[] };
    expect(config.branches[0].transform.steps.map((s) => s.kind)).toEqual(['mask', 'js']);
    expect(plan.request.events?.[0]).toMatchObject({ key: 'k7', value: { email: 'a@b.c' }, topic: 'orders' });
    expect(plan.without(steps, 0, 'bad').stepIndexes).toEqual([2]);
  });

  type RunDoc = { source: { connectionId: string }; branches: { destination: { connectionId: string } }[] };

  it('names the sample session and the real source connection (no placeholder id)', () => {
    const source = { sessionId: 'ps_42', connectionId: '6f1c2a9e-1111-4222-8333-944455556666' };
    const plan = new StudioRunPlan(steps, samples, source);
    const config = plan.request.config as RunDoc;
    expect(plan.request.sampleSessionId).toBe('ps_42');
    expect(config.source.connectionId).toBe(source.connectionId);
    expect(config.branches[0].destination.connectionId).toBe(source.connectionId);
    expect(JSON.stringify(plan.request)).not.toContain('00000000-0000-0000-0000-000000000000');
    expect(plan.without(steps, 0, 'bad').request.sampleSessionId).toBe('ps_42');
  });

  it('sends an upload session without a connection: no connection to mask by', () => {
    const plan = new StudioRunPlan(steps, samples, { sessionId: 'ps_up', connectionId: null });
    expect(plan.request.sampleSessionId).toBe('ps_up');
    expect((plan.request.config as RunDoc).source.connectionId).toBe(StudioRunDocument.unboundConnectionId);
    expect(new StudioRunPlan(steps, samples).request).not.toHaveProperty('sampleSessionId');
  });
});

describe('StudioRunResult', () => {
  const result = new StudioRunResult(new StudioRunPlan(steps, samples), runResult);

  it('reports failures with draft step index and JS location', () => {
    const run = result.run(7);
    expect(run?.error).toEqual({ stepIndex: 2, message: expect.stringContaining('TypeError'), line: 1, column: 31 });
    expect(result.resultFor(7)).toMatchObject({ error: expect.any(String), stepIndex: 2, logs: ['hi'] });
    expect(result.resultFor(7)?.event?.value).toEqual({ email: '*@b.c', a: 1 });
  });

  it('shows the event at a selected step and the drop after it', () => {
    expect(result.resultFor(9, 0)?.event?.value).toEqual({ email: '*@y.z', a: 2 });
    expect(result.resultFor(9)).toMatchObject({ dropped: true, stepIndex: 2 });
    expect(result.snapshots(9, 0).before?.value).toEqual({ email: 'x@y.z', a: 2 });
    expect(result.snapshots(9, 0).after?.value).toEqual({ email: '*@y.z', a: 2 });
  });

  it('carries skipped steps as issues', () => {
    expect(result.issues.get(1)).toBeTruthy();
  });
});

/** Preview double: records requests, answers from a queue of results or failures. */
class FakePreview extends PreviewService {
  readonly requests: RunRequest[] = [];
  readonly answers: (RunResult | Error)[] = [];
  loadSamples = vi.fn();
  listEvents = vi.fn();
  schema = vi.fn();
  testRun = vi.fn();
  close = vi.fn();

  run(request: RunRequest, signal?: AbortSignal): Promise<RunResult> {
    this.requests.push(request);
    const answer = this.answers.shift() ?? runResult;
    return new Promise((resolve, reject) => {
      signal?.addEventListener('abort', () => reject(Object.assign(new Error('aborted'), { name: 'AbortError' })));
      queueMicrotask(() => (answer instanceof Error ? reject(answer) : resolve(answer)));
    });
  }

  failure(error: unknown): PreviewFailure {
    const e = error as Error & { path?: string; kind?: PreviewFailureKind };
    if (e.name === 'AbortError') return { kind: PreviewFailureKind.Aborted, message: e.message };
    return { kind: e.kind ?? PreviewFailureKind.Other, message: e.message, path: e.path };
  }
}

describe('StudioPreviewController', () => {
  let preview: FakePreview;
  let states: StudioPreviewState[];
  let controller: StudioPreviewController;
  beforeEach(() => {
    vi.useFakeTimers();
    preview = new FakePreview();
    states = [];
    controller = new StudioPreviewController(preview, (s) => states.push(s));
  });
  afterEach(() => vi.useRealTimers());

  it('debounces edits: only the last one within 300 ms runs', async () => {
    controller.schedule(steps, samples);
    controller.schedule(steps.slice(0, 1), samples);
    await vi.advanceTimersByTimeAsync(299);
    expect(preview.requests).toHaveLength(0);
    await vi.advanceTimersByTimeAsync(1);
    expect(preview.requests).toHaveLength(1);
    expect(states.at(-1)?.result).not.toBeNull();
  });

  it('discards a stale response when a newer edit arrived', async () => {
    controller.schedule(steps, samples, true);
    controller.schedule(steps.slice(0, 1), samples, true);
    await vi.runAllTimersAsync();
    expect(preview.requests).toHaveLength(2);
    expect(states.filter((s) => s.failure)).toHaveLength(0);
    expect(states.at(-1)?.pending).toBe(false);
  });

  it('leaves out a step the preview rejects and keeps previewing the others', async () => {
    preview.answers.push(Object.assign(new Error('missing property to'), { kind: PreviewFailureKind.InvalidConfig, path: '/config/branches/0/transform/steps/0' }));
    controller.schedule(steps, samples, true);
    await vi.runAllTimersAsync();
    expect(preview.requests).toHaveLength(2);
    const final = states.at(-1);
    expect(final?.result?.issues.get(0)).toBe('missing property to');
    expect(final?.failure).toBeNull();
  });

  it('reports other failures and keeps the last good result', async () => {
    controller.schedule(steps, samples, true);
    await vi.runAllTimersAsync();
    preview.answers.push(new Error('preview down'));
    controller.schedule(steps, samples, true);
    const settled = await controller.settled();
    expect(settled.failure?.message).toBe('preview down');
    expect(settled.result).not.toBeNull();
  });

  it('sends the sample source with every run, including retries without a rejected step', async () => {
    preview.answers.push(Object.assign(new Error('missing property to'), { kind: PreviewFailureKind.InvalidConfig, path: '/config/branches/0/transform/steps/0' }));
    controller.schedule(steps, samples, true, { sessionId: 'ps_7', connectionId: null });
    await vi.runAllTimersAsync();
    expect(preview.requests.map((r) => r.sampleSessionId)).toEqual(['ps_7', 'ps_7']);
  });

  it('does not call the preview without samples', () => {
    controller.schedule(steps, []);
    expect(preview.requests).toHaveLength(0);
    expect(states.at(-1)).toEqual({ result: null, pending: false, failure: null });
  });
});
