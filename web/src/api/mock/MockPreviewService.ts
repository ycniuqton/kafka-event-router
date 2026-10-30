import type { RouteConfig } from '../generated';
import {
  EventStatus,
  FilterEvaluator,
  FilterMode,
  FilterTrace,
  JsonPatchDiffer,
  LookupMockPlan,
  PreviewFailureKind,
  RawEncoding,
  RouteStatus,
  SampleRecords,
  StopReason,
  TransformRunner,
  type BranchResult,
  type EventResult,
  type FieldNode,
  type FieldSchema,
  type FilterNodeResult,
  type InlineEvent,
  type JsonValue,
  type KafkaEvent,
  type LookupMocks,
  type LoadSamplesRequest,
  type PreviewFailure,
  type RouteDraft,
  type RunRequest,
  type RunResult,
  type SampleEvent,
  type SampleLoad,
  type SamplePage,
  type SampleQuery,
  type SampleRecord,
  type StepTrace,
  type TestTopic,
  type ValueFormat,
} from '../../domain';
import { RouteDraftMapper } from '../../domain/routeConfig/RouteDraftMapper';
import { SAMPLES, SampleSchema } from '../../mocks';
import { PreviewService } from '../contracts';
import type { MockBackend } from './MockBackend';

/** Constants of the in-memory preview sessions. */
const MockPreviewPolicy = {
  idPrefix: 'ps_mock_',
  ttlMs: 3_600_000,
  testTopicTtlMs: 86_400_000,
  testTopicPrefix: 'router.test.',
  stampLength: 14,
  unsafeTopicChars: /[^a-z0-9._-]+/g,
  stampSeparators: /[-:T]/g,
  topicFill: '-',
  uploadTopic: 'upload',
  mainBranch: 'main',
  previewName: 'preview',
  arrayOpen: '[',
  lineBreak: '\n',
} as const;

/** Builds the test topic of a mock test run (same naming as the server: router.test.<route>.<yyyymmddhhmmss>). */
class MockTestTopics {
  static of(config: unknown, produced: number, now: Date): TestTopic {
    const doc = (config ?? {}) as { name?: string; branches?: { destination?: { connectionId?: string } }[] };
    const slug = (doc.name ?? MockPreviewPolicy.previewName).toLowerCase().replace(MockPreviewPolicy.unsafeTopicChars, MockPreviewPolicy.topicFill);
    const stamp = now.toISOString().replace(MockPreviewPolicy.stampSeparators, '').slice(0, MockPreviewPolicy.stampLength);
    const expiresAt = new Date(now.getTime() + MockPreviewPolicy.testTopicTtlMs).toISOString();
    return { connectionId: doc.branches?.[0]?.destination?.connectionId ?? '', topic: `${MockPreviewPolicy.testTopicPrefix}${slug}.${stamp}`, produced, expiresAt };
  }
}

/** Converts mock order events and inline uploads into preview records. */
class MockRecordFactory {
  static fromSample(ev: SampleEvent, seq: number): SampleRecord {
    const value = ev.value as unknown as JsonValue;
    return {
      seq,
      topic: ev.topic,
      partition: ev.partition,
      offset: ev.offset,
      timestamp: ev.timestamp,
      key: ev.key,
      value,
      headers: Object.entries(ev.headers).map(([key, v]) => ({ key, value: v })),
      raw: { key: ev.key, value: JSON.stringify(value), encoding: RawEncoding.Utf8 },
      decodeError: null,
    };
  }

  /** JSON array or NDJSON; items with a `value` member are events, others become the value. */
  static parseUpload(text: string): InlineEvent[] {
    const trimmed = text.trim();
    const items: JsonValue[] = trimmed.startsWith(MockPreviewPolicy.arrayOpen) ? JSON.parse(trimmed) : trimmed.split(MockPreviewPolicy.lineBreak).filter((l) => l.trim()).map((l) => JSON.parse(l));
    return items.map((item) => (item && typeof item === 'object' && !Array.isArray(item) && 'value' in item ? (item as InlineEvent) : { value: item }));
  }

  static fromInline(ev: InlineEvent, seq: number, now: string): SampleRecord {
    const key = ev.key ?? null;
    return {
      seq,
      topic: ev.topic ?? MockPreviewPolicy.uploadTopic,
      partition: -1,
      offset: seq,
      timestamp: ev.timestamp ?? now,
      key,
      value: ev.value,
      headers: ev.headers ?? [],
      raw: { key: key === null ? null : String(key), value: JSON.stringify(ev.value), encoding: RawEncoding.Utf8 },
      decodeError: null,
    };
  }
}

/** Puts the run's `lookupMocks` back on the draft's lookup source steps (matched by the run document's step ids). */
class MockLookupMocks {
  static apply(draft: RouteDraft, config: RouteConfig, mocks: LookupMocks): RouteDraft {
    const ids = config.branches?.[0]?.transform?.steps?.map((s) => s.id) ?? [];
    const steps = draft.steps.map((step, i) => {
      const mock = ids[i] ? mocks[ids[i] as string] : undefined;
      return mock && step.lookupSource ? { ...step, lookupSource: LookupMockPlan.applyTo(step.lookupSource, mock) } : step;
    });
    return { ...draft, steps };
  }
}

/** Runs a draft config on records with the browser pipeline, emitting the server's RunResult shape. */
class MockPreviewRunner {
  run(config: unknown, records: SampleRecord[], mocks: LookupMocks = {}): RunResult {
    const draft = MockLookupMocks.apply(MockPreviewRunner.draftOf(config), config as RouteConfig, mocks);
    const results = records.map((r) => this.runOne(draft, r));
    const count = (s: EventStatus) => results.filter((r) => r.status === s).length;
    return {
      summary: {
        loaded: records.length,
        processed: records.length,
        passed: count(EventStatus.Passed),
        filtered: count(EventStatus.Filtered),
        errors: count(EventStatus.Error),
        dropped: count(EventStatus.Dropped),
        incomplete: false,
        durationMs: 0,
        branches: [{ branch: MockPreviewPolicy.mainBranch, passed: count(EventStatus.Passed), filtered: count(EventStatus.Filtered), errors: count(EventStatus.Error), dropped: count(EventStatus.Dropped) }],
      },
      issues: [],
      results,
    };
  }

  private static draftOf(config: unknown): RouteDraft {
    const meta = { routeId: null, name: MockPreviewPolicy.previewName, status: RouteStatus.Draft };
    return RouteDraftMapper.fromDocument(config as RouteConfig, meta).draft;
  }

  private runOne(draft: RouteDraft, record: SampleRecord): EventResult {
    const event = SampleRecords.toEvent(record);
    const verdict = FilterEvaluator.evaluate(event, draft.filter);
    const filter = this.filterNodes(draft, verdict.rowResults, verdict.pass);
    if (verdict.error) return this.result(record.seq, EventStatus.Error, filter, [], verdict.error);
    if (!verdict.pass) return this.result(record.seq, EventStatus.Filtered, filter, []);
    const steps = this.steps(draft, event);
    const run = TransformRunner.run(event, draft.steps);
    if (run.error) return this.result(record.seq, EventStatus.Error, filter, steps, run.error, run.stepIndex);
    if (run.dropped) return this.result(record.seq, EventStatus.Dropped, filter, steps);
    return this.result(record.seq, EventStatus.Passed, filter, steps, undefined, undefined, run.event);
  }

  private filterNodes(draft: RouteDraft, rows: boolean[], pass: boolean): FilterNodeResult[] {
    const root = { path: FilterTrace.branchFilter, result: pass };
    if (draft.filter.mode === FilterMode.Expression || rows.length <= 1) return [root];
    return [...rows.map((result, i) => ({ path: FilterTrace.rowPointer(i, rows.length), result })), root];
  }

  private steps(draft: RouteDraft, input: KafkaEvent): StepTrace[] {
    const out: StepTrace[] = [];
    let prev = MockPreviewRunner.doc(input);
    draft.steps.forEach((step, i) => {
      const run = TransformRunner.run(input, draft.steps, i);
      const next = run.event ? MockPreviewRunner.doc(run.event) : prev;
      out.push({ index: i, id: `step-${i}`, kind: step.kind, patch: JsonPatchDiffer.diff(prev, next), durationUs: 0, logs: run.logs, error: run.error ?? null });
      prev = next;
    });
    return out;
  }

  private result(seq: number, status: EventStatus, filter: FilterNodeResult[], steps: StepTrace[], message?: string, stepIndex?: number, output?: KafkaEvent): EventResult {
    const error = message ? { stepIndex: stepIndex ?? null, errorType: status, message } : null;
    const outputs = output ? [MockPreviewRunner.output(output)] : [];
    const branch: BranchResult = { branch: MockPreviewPolicy.mainBranch, status, filter, steps, error, outputs };
    return { seq, status, error, branches: [branch] };
  }

  private static doc(ev: KafkaEvent): JsonValue {
    return { key: ev.key, value: ev.value, headers: Object.entries(ev.headers).map(([key, value]) => ({ key, value })) };
  }

  private static output(ev: KafkaEvent): BranchResult['outputs'][number] {
    const headers = Object.entries(ev.headers).map(([key, value]) => ({ key, value }));
    const value = ev.value as JsonValue;
    return { topic: '', key: ev.key, value, headers, sizeBytes: JSON.stringify(value).length, valid: true, validationError: null };
  }
}

/** In-memory live preview: sessions from the design's order events or uploads, runs on the browser pipeline. */
export class MockPreviewService extends PreviewService {
  private readonly backend: MockBackend;
  private readonly sessions = new Map<string, SampleRecord[]>();
  private readonly runner = new MockPreviewRunner();
  private nextId = 0;

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  loadSamples(request: LoadSamplesRequest): Promise<SampleLoad> {
    const now = this.backend.nowIso();
    const inline = request.upload ? MockRecordFactory.parseUpload(request.upload) : request.events;
    const records = inline
      ? inline.map((e, i) => MockRecordFactory.fromInline(e, i, now))
      : SAMPLES.slice(-(request.count ?? SAMPLES.length)).map((e, i) => MockRecordFactory.fromSample(e, i));
    const id = `${MockPreviewPolicy.idPrefix}${this.nextId++}`;
    this.sessions.set(id, records);
    const expiresAt = new Date(this.backend.clock.now().getTime() + MockPreviewPolicy.ttlMs).toISOString();
    const session = { id, expiresAt, loaded: records.length, bytes: 0, truncated: false, stopReason: StopReason.Complete, masked: false };
    return this.backend.reply({ session, events: records });
  }

  listEvents(sessionId: string, query: SampleQuery = {}): Promise<SamplePage> {
    const text = query.q?.toLowerCase() ?? '';
    const records = (this.sessions.get(sessionId) ?? []).filter((r) => !text || JSON.stringify(r).toLowerCase().includes(text));
    return this.backend.reply({ events: records });
  }

  schema(): Promise<FieldSchema> {
    const children: FieldNode[] = SampleSchema.fields.map((path) => ({
      path,
      types: { [SampleSchema.isNumeric(path) ? 'number' : 'string']: SAMPLES.length },
      present: SAMPLES.length,
      frequency: 1,
      topValues: [],
      children: [],
    }));
    return this.backend.reply({ samples: SAMPLES.length, root: { path: '', types: {}, present: SAMPLES.length, frequency: 1, topValues: [], children } });
  }

  run(request: RunRequest): Promise<RunResult> {
    return this.backend.reply(this.runner.run(request.config, this.records(request), request.lookupMocks));
  }

  testRun(request: RunRequest): Promise<RunResult> {
    const result = this.runner.run(request.config, this.records(request), request.lookupMocks);
    const testTopic = request.writeToTopic ? MockTestTopics.of(request.config, result.summary.passed, this.backend.clock.now()) : undefined;
    return this.backend.reply({ ...result, testTopic });
  }

  reformat(sessionId: string, format: ValueFormat): Promise<SampleLoad> {
    const records = this.sessions.get(sessionId) ?? [];
    const expiresAt = new Date(this.backend.clock.now().getTime() + MockPreviewPolicy.ttlMs).toISOString();
    const session = { id: sessionId, expiresAt, loaded: records.length, bytes: 0, truncated: false, stopReason: StopReason.Complete, masked: false, format };
    return this.backend.reply({ session, events: records });
  }

  async close(sessionId: string): Promise<void> {
    this.sessions.delete(sessionId);
  }

  failure(error: unknown): PreviewFailure {
    return { kind: PreviewFailureKind.Other, message: error instanceof Error ? error.message : String(error) };
  }

  private records(request: RunRequest): SampleRecord[] {
    if (request.events) return request.events.map((e, i) => MockRecordFactory.fromInline(e, i, this.backend.nowIso()));
    const all = this.sessions.get(request.sessionId ?? '') ?? [];
    return request.eventSeqs ? all.filter((r) => request.eventSeqs?.includes(r.seq)) : all;
  }
}
