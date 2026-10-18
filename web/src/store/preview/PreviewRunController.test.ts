import { describe, expect, it } from 'vitest';
import {
  PreviewFailureKind,
  RawEncoding,
  StopReason,
  type LoadSamplesRequest,
  type PreviewFailure,
  type RunRequest,
  type RunResult,
  type SampleLoad,
  type SamplePage,
  type ValueFormat,
} from '../../domain';
import { PreviewService } from '../../api/contracts';
import { PreviewActions } from './PreviewActions';
import { PreviewPhase } from './PreviewState';
import { PreviewStore } from './PreviewStore';
import type { RunTimers } from './PreviewRunController';

/** Fake preview service: scripted run outcomes, counts loads and records aborted runs. */
class FakePreviewService extends PreviewService {
  loads = 0;
  runs: RunRequest[] = [];
  aborted = 0;
  outcomes: (RunResult | PreviewFailure)[] = [];

  loadSamples(request: LoadSamplesRequest): Promise<SampleLoad> {
    this.loads++;
    const record = { seq: 0, topic: 't', partition: 0, offset: 0, timestamp: '', key: null, value: { n: 1 }, headers: [], raw: { key: null, value: '{}', encoding: RawEncoding.Utf8 }, decodeError: null };
    const session = { id: `ps_${this.loads}`, expiresAt: '', loaded: 1, bytes: 2, truncated: false, stopReason: StopReason.Complete, masked: false };
    return Promise.resolve({ session, events: request.count === 0 ? [] : [record] });
  }
  listEvents(): Promise<SamplePage> {
    return Promise.resolve({ events: [] });
  }
  schema(): Promise<never> {
    return Promise.reject(new Error('no schema'));
  }
  run(request: RunRequest, signal?: AbortSignal): Promise<RunResult> {
    this.runs.push(request);
    signal?.addEventListener('abort', () => this.aborted++);
    const next = this.outcomes.shift() ?? FakePreviewService.ok();
    return 'kind' in next ? Promise.reject(next) : Promise.resolve(next);
  }
  testRun(request: RunRequest): Promise<RunResult> {
    return this.run(request);
  }
  close(): Promise<void> {
    return Promise.resolve();
  }
  reformats: ValueFormat[] = [];
  reformat(sessionId: string, format: ValueFormat): Promise<SampleLoad> {
    this.reformats.push(format);
    const record = { seq: 0, topic: 't', partition: 0, offset: 0, timestamp: '', key: null, value: { avro: true }, headers: [], raw: { key: null, value: '', encoding: RawEncoding.Base64 }, decodeError: null };
    const session = { id: sessionId, expiresAt: '', loaded: 1, bytes: 2, truncated: false, stopReason: StopReason.Complete, masked: false, format };
    return Promise.resolve({ session, events: [record] });
  }
  failure(error: unknown): PreviewFailure {
    return error as PreviewFailure;
  }
  static ok(): RunResult {
    return { summary: { loaded: 1, processed: 1, passed: 1, filtered: 0, errors: 0, dropped: 0, incomplete: false, durationMs: 1 }, issues: [], results: [] };
  }
}

/** Manual timers: pending callbacks run on `flush()`. */
class ManualTimers implements RunTimers {
  private pending = new Map<number, () => void>();
  private next = 0;
  set = (fn: () => void) => {
    this.pending.set(++this.next, fn);
    return this.next;
  };
  clear = (handle: unknown) => {
    this.pending.delete(handle as number);
  };
  flush(): void {
    const fns = [...this.pending.values()];
    this.pending.clear();
    fns.forEach((fn) => fn());
  }
  get size(): number {
    return this.pending.size;
  }
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 0));
const source = { connectionId: 'c1', topics: ['orders'] };

function setup() {
  const service = new FakePreviewService();
  const timers = new ManualTimers();
  const actions = new PreviewActions(new PreviewStore(), service, timers);
  return { service, timers, actions, state: () => actions.store.getState() };
}

describe('PreviewRunController', () => {
  it('debounces draft edits into one run with the latest config', async () => {
    const { service, timers, actions, state } = setup();
    await actions.loadLatest(source);
    actions.scheduleRun({ v: 1 });
    actions.scheduleRun({ v: 2 });
    expect(timers.size).toBe(1);
    timers.flush();
    await settle();
    expect(service.runs.map((r) => r.config)).toEqual([{ v: 2 }]);
    expect(state().runPhase).toBe(PreviewPhase.Ready);
  });

  it('aborts a stale in-flight run when a newer one starts', async () => {
    const { service, actions } = setup();
    await actions.loadLatest(source);
    const first = actions.runNow({ v: 1 });
    const second = actions.runNow({ v: 2 });
    expect(service.aborted).toBe(1);
    expect(await first).toBeNull();
    expect(await second).not.toBeNull();
  });

  it('reloads an expired session silently and re-runs once', async () => {
    const { service, actions, state } = setup();
    await actions.loadLatest(source);
    service.outcomes = [{ kind: PreviewFailureKind.SessionExpired, message: 'gone' }];
    const result = await actions.runNow({ v: 1 });
    expect(result).not.toBeNull();
    expect(service.loads).toBe(2);
    expect(service.runs.map((r) => r.sessionId)).toEqual(['ps_1', 'ps_2']);
    expect(state().runFailure).toBeNull();
  });

  it('retries once when busy, then shows the failure', async () => {
    const { service, timers, actions, state } = setup();
    await actions.loadLatest(source);
    const busy = { kind: PreviewFailureKind.Busy, message: 'busy' };
    service.outcomes = [busy, busy];
    const run = actions.runNow({ v: 1 });
    await settle();
    timers.flush();
    expect(await run).toBeNull();
    expect(service.runs).toHaveLength(2);
    expect(state().runFailure?.kind).toBe(PreviewFailureKind.Busy);
  });

  it('keeps config errors for inline display', async () => {
    const { service, actions, state } = setup();
    await actions.loadLatest(source);
    service.outcomes = [{ kind: PreviewFailureKind.InvalidConfig, message: 'bad', path: '/branches/0/filter' }];
    await actions.runNow({});
    expect(state().runPhase).toBe(PreviewPhase.Failed);
    expect(state().runFailure?.path).toBe('/branches/0/filter');
  });

  it('does not run before samples are loaded', async () => {
    const { service, actions } = setup();
    expect(await actions.runNow({})).toBeNull();
    expect(service.runs).toHaveLength(0);
  });
});

describe('P2 panel actions', () => {
  const source = { connectionId: 'c1', topics: ['orders'] };

  it('reformat re-decodes the session and survives an ensure-load of the same source', async () => {
    const service = new FakePreviewService();
    const store = new PreviewStore();
    const actions = new PreviewActions(store, service, new ManualTimers());
    await actions.ensureLatest(source);
    expect(await actions.reformat('avro')).toBe(true);
    expect(service.reformats).toEqual(['avro']);
    expect(store.getState().session?.format).toBe('avro');
    expect(store.getState().records[0].value).toEqual({ avro: true });
    await actions.ensureLatest(source);
    expect(service.loads).toBe(1);
  });

  it('writes the test topic through a test run with writeToTopic', async () => {
    const service = new FakePreviewService();
    const store = new PreviewStore();
    const actions = new PreviewActions(store, service, new ManualTimers());
    await actions.ensureLatest(source);
    await actions.writeTestTopic({ name: 'r' }, 'route-1');
    expect(service.runs.at(-1)).toMatchObject({ writeToTopic: true, routeId: 'route-1' });
    expect(store.getState().testPhase).toBe(PreviewPhase.Ready);
  });

  it('remembers the selected branch', () => {
    const store = new PreviewStore();
    const actions = new PreviewActions(store, new FakePreviewService(), new ManualTimers());
    actions.setBranch('refunds');
    expect(store.getState().branch).toBe('refunds');
  });
});
