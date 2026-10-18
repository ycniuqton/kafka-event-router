import { describe, expect, it } from 'vitest';
import {
  ClusterKind,
  DemoSamples,
  SampleMode,
  SampleSummary,
  StopReason,
  type FieldSchema,
  type LoadSamplesRequest,
  type PreviewFailure,
  type RunResult,
  type SampleLoad,
  type SamplePage,
  type ValueFormat,
} from '../../domain';
import { PreviewService } from '../../api/contracts';
import { PreviewActions } from './PreviewActions';
import { PreviewStore } from './PreviewStore';

/** Records load requests; every load returns an empty session. */
class RecordingPreviewService extends PreviewService {
  requests: LoadSamplesRequest[] = [];
  loadSamples(request: LoadSamplesRequest): Promise<SampleLoad> {
    this.requests.push(request);
    const session = { id: `ps_${this.requests.length}`, expiresAt: '', loaded: 1, bytes: 1, truncated: false, stopReason: StopReason.Complete, masked: false };
    return Promise.resolve({ session, events: [] });
  }
  listEvents(): Promise<SamplePage> {
    return Promise.resolve({ events: [] });
  }
  schema(): Promise<FieldSchema> {
    return Promise.reject(new Error('no schema'));
  }
  run(): Promise<RunResult> {
    return Promise.reject(new Error('no run'));
  }
  testRun(): Promise<RunResult> {
    return Promise.reject(new Error('no run'));
  }
  close(): Promise<void> {
    return Promise.resolve();
  }
  reformat(_id: string, _format: ValueFormat): Promise<SampleLoad> {
    return Promise.reject(new Error('no reformat'));
  }
  failure(error: unknown): PreviewFailure {
    return error as PreviewFailure;
  }
}

describe('DemoSamples', () => {
  it('applies only to a demo connection reading order-events', () => {
    expect(DemoSamples.applies({ kind: ClusterKind.Demo }, ['order-events'])).toBe(true);
    expect(DemoSamples.applies({ kind: ClusterKind.Demo }, ['order-events', 'other'])).toBe(false);
    expect(DemoSamples.applies({ kind: ClusterKind.Demo }, ['payments'])).toBe(false);
    expect(DemoSamples.applies({ kind: ClusterKind.Kafka }, ['order-events'])).toBe(false);
    expect(DemoSamples.applies(undefined, ['order-events'])).toBe(false);
  });
});

describe('PreviewActions demo source', () => {
  it('loads the demo source for a demo sandbox and Kafka latest otherwise', async () => {
    const service = new RecordingPreviewService();
    const actions = new PreviewActions(new PreviewStore(), service);
    await actions.ensureLatest({ connectionId: 'demo-1', topics: ['order-events'], demo: true });
    await actions.ensureLatest({ connectionId: 'demo-1', topics: ['order-events'], demo: true });
    await actions.loadLatest({ connectionId: 'demo-1', topics: ['order-events'], demo: true }, 200);
    await actions.ensureLatest({ connectionId: 'kafka-1', topics: ['orders'] });
    expect(service.requests.map((r) => [r.mode, r.count])).toEqual([
      [SampleMode.Demo, 50],
      [SampleMode.Demo, 200],
      [SampleMode.Latest, 50],
    ]);
  });

  it('shows the demo notice for demo sessions only', async () => {
    const store = new PreviewStore();
    const actions = new PreviewActions(store, new RecordingPreviewService());
    await actions.ensureLatest({ connectionId: 'demo-1', topics: ['order-events'], demo: true });
    expect(SampleSummary.demoNotice(store.getState().request)).toBe(DemoSamples.notice);
    await actions.ensureLatest({ connectionId: 'kafka-1', topics: ['orders'] });
    expect(SampleSummary.demoNotice(store.getState().request)).toBeNull();
  });
});
