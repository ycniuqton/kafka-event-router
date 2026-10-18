import {
  ClusterKind,
  ConnectionStatus,
  DemoDataText,
  EnvTag,
  RouteStatus,
  TailCloseCode,
  TailEncoding,
  TailFrameType,
  TailOp,
  TailOutcome,
  TailState,
  type Connection,
  type DemoSeed,
  type OnboardingState,
  type SampleEvent,
  type TailClientOp,
  type TailOutcome as TailOutcomeT,
  type TailRecord,
} from '../../domain';
import { SAMPLES } from '../../mocks';
import { LiveTailService, OnboardingService, type TailChannel, type TailHandlers } from '../contracts';
import type { MockBackend } from './MockBackend';

/** Pace of the mock tail and the outcome cycle of its synthetic records. */
export const MockTailTiming = { intervalMs: 700, openDelayMs: 50 } as const;
const OutcomeCycle = { filteredEvery: 4, deadLetterEvery: 9, errorEvery: 13, offsetStride: 3 } as const;

/** Builds deterministic tail records from the sample order events. */
export class MockTailRecords {
  static outcome(seq: number): TailOutcomeT {
    if (seq % OutcomeCycle.errorEvery === OutcomeCycle.errorEvery - 1) return TailOutcome.Error;
    if (seq % OutcomeCycle.deadLetterEvery === OutcomeCycle.deadLetterEvery - 1) return TailOutcome.DeadLettered;
    if (seq % OutcomeCycle.filteredEvery === OutcomeCycle.filteredEvery - 1) return TailOutcome.Filtered;
    return TailOutcome.Sent;
  }

  static record(seq: number, at: string, destination: string): TailRecord {
    const sample: SampleEvent = SAMPLES[seq % SAMPLES.length];
    const outcome = MockTailRecords.outcome(seq);
    const offset = sample.offset + seq * OutcomeCycle.offsetStride;
    const input = { topic: sample.topic, partition: sample.partition, offset, key: sample.key, value: sample.value, encoding: TailEncoding.Json };
    const base = { at, partition: sample.partition, offset, outcome, input, outputs: [] };
    if (outcome === TailOutcome.Error) return { ...base, step: 'transform[1]', error: 'amount: expected number' };
    if (outcome === TailOutcome.DeadLettered) return { ...base, errorType: 'deserialization', error: 'invalid JSON at byte 0' };
    if (outcome !== TailOutcome.Sent) return base;
    const value = { orderId: sample.value.orderId, email: sample.value.email, amount: sample.value.amount };
    return { ...base, outputs: [{ topic: destination, partition: sample.partition, offset, encoding: TailEncoding.Json, value }] };
  }
}

/** One simulated tail socket: emits a record per tick while live, honours pause/filter like the server. */
class MockTailChannel implements TailChannel {
  private timer: ReturnType<typeof setInterval> | undefined;
  private paused = false;
  private outcomes: TailOutcomeT[] = [];
  private text = '';
  private seq = 0;
  private readonly backend: MockBackend;
  private readonly handlers: TailHandlers;
  private readonly destination: string;

  constructor(backend: MockBackend, handlers: TailHandlers, destination: string) {
    this.backend = backend;
    this.handlers = handlers;
    this.destination = destination;
  }

  start(): void {
    this.handlers.onOpen?.();
    this.handlers.onFrame({ type: TailFrameType.Status, state: TailState.Live, masked: false });
    this.timer = setInterval(() => this.tick(), MockTailTiming.intervalMs);
  }

  send(op: TailClientOp): boolean {
    if (op.op === TailOp.Filter) {
      this.outcomes = op.outcomes ?? [];
      this.text = (op.text ?? '').toLowerCase();
      return true;
    }
    this.paused = op.op === TailOp.Pause;
    this.handlers.onFrame({ type: TailFrameType.Status, state: this.paused ? TailState.Paused : TailState.Live, masked: false });
    return true;
  }

  close(): void {
    clearInterval(this.timer);
  }

  private tick(): void {
    const record = MockTailRecords.record(this.seq++, this.backend.nowIso(), this.destination);
    if (this.paused || !this.matches(record)) return;
    this.handlers.onFrame({ type: TailFrameType.Record, record, masked: false });
  }

  private matches(record: TailRecord): boolean {
    if (this.outcomes.length > 0 && !this.outcomes.includes(record.outcome)) return false;
    return !this.text || JSON.stringify(record).toLowerCase().includes(this.text);
  }
}

/** `services.liveTail` on mocks: timer-driven records for running routes; others close with 4409. */
export class MockLiveTailService extends LiveTailService {
  private readonly backend: MockBackend;

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  open(routeId: string, handlers: TailHandlers): TailChannel {
    const route = this.backend.db.routes.find((r) => r.id === routeId);
    const channel = new MockTailChannel(this.backend, handlers, route?.destination.topic ?? '');
    const running = route?.status === RouteStatus.Running || route?.status === RouteStatus.Error;
    setTimeout(() => (running ? channel.start() : handlers.onClose(TailCloseCode.NotRunning)), MockTailTiming.openDelayMs);
    return channel;
  }
}

/** `services.onboarding` on mocks: the demo connection is an in-memory `kind=demo` entry. */
export class MockOnboardingService extends OnboardingService {
  static readonly demoName = 'demo-orders';
  private readonly backend: MockBackend;

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  get(): Promise<OnboardingState> {
    const demo = this.demo();
    return this.backend.reply({ allDone: false, demoAvailable: true, demo: demo ? this.seed(demo) : null });
  }

  createDemo(): Promise<DemoSeed> {
    if (this.demo()) return this.backend.fail(DemoDataText.exists);
    const connection: Connection = {
      id: MockOnboardingService.demoName, name: MockOnboardingService.demoName, env: EnvTag.Dev, bootstrapServers: ['demo-cluster:9092'],
      security: 'None', status: ConnectionStatus.Healthy, checkedAt: this.backend.nowIso(), owner: 'Platform', kind: ClusterKind.Demo,
      topics: [{ name: DemoDataText.topic, partitions: 3, rate: '12/s' }],
    };
    this.backend.db.connections = [...this.backend.db.connections, connection];
    return this.backend.reply(this.seed(connection));
  }

  removeDemo(): Promise<void> {
    this.backend.db.connections = this.backend.db.connections.filter((c) => c.kind !== ClusterKind.Demo);
    return this.backend.reply(undefined);
  }

  private demo(): Connection | undefined {
    return this.backend.db.connections.find((c) => c.kind === ClusterKind.Demo);
  }

  private seed(c: Connection): DemoSeed {
    return { connectionId: c.id, topic: DemoDataText.topic };
  }
}
