import { ReplayStatus, ReplayRules, type Replay, type ReplayRequest } from '../../domain';
import type { MockBackend } from './MockBackend';

/** Messages of the mock replay backend (same wording as the API's problems). */
export const MockReplayText = {
  confirm: 'Confirm the replay: the destination will receive duplicates of events it already got.',
  future: 'The replay start is in the future. Pick a time in the past.',
  invalid: 'Pick the date and time to replay from.',
  running: 'A replay of this route is already running. Wait until it finishes.',
  notFound: 'This replay does not exist.',
} as const;

/** Simulated replays: one open replay per route that walks pending → stopping → resetting → done over time. */
export class MockRouteReplays {
  static readonly stepMs = 1_000;
  static readonly mockPartitions = 3;
  private static readonly steps = [ReplayStatus.Pending, ReplayStatus.Stopping, ReplayStatus.Resetting, ReplayStatus.Done];

  private readonly backend: MockBackend;
  private readonly replays = new Map<string, Replay>();
  private sequence = 0;

  constructor(backend: MockBackend) {
    this.backend = backend;
  }

  request(routeId: string, request: ReplayRequest): Promise<Replay> {
    const problem = this.problem(routeId, request);
    if (problem) return this.backend.fail(problem);
    this.sequence += 1;
    const now = this.backend.clock.now().toISOString();
    const replay: Replay = { id: `rp-${routeId}-${this.sequence}`, routeId, status: ReplayStatus.Pending, from: new Date(request.from).toISOString(),
      requestedAt: now, startedAt: null, finishedAt: null, error: '', partitions: 0 };
    this.replays.set(replay.id, replay);
    return this.backend.reply(replay);
  }

  status(routeId: string, replayId: string): Promise<Replay> {
    const replay = this.replays.get(replayId);
    if (!replay || replay.routeId !== routeId) return this.backend.fail(MockReplayText.notFound);
    return this.backend.reply(this.advance(replay));
  }

  /** The first rule a request breaks, or null. */
  private problem(routeId: string, request: ReplayRequest): string | null {
    const from = new Date(request.from).getTime();
    if (!request.confirm) return MockReplayText.confirm;
    if (Number.isNaN(from)) return MockReplayText.invalid;
    if (from > this.backend.clock.now().getTime()) return MockReplayText.future;
    const open = [...this.replays.values()].some((r) => r.routeId === routeId && !ReplayRules.finished(this.advance(r).status));
    return open ? MockReplayText.running : null;
  }

  /** Status derived from the time since the request. */
  private advance(replay: Replay): Replay {
    const elapsed = this.backend.clock.now().getTime() - new Date(replay.requestedAt).getTime();
    const index = Math.min(Math.floor(elapsed / MockRouteReplays.stepMs), MockRouteReplays.steps.length - 1);
    const status = MockRouteReplays.steps[index];
    const done = ReplayRules.finished(status);
    const next: Replay = { ...replay, status, startedAt: index > 0 ? replay.requestedAt : null,
      finishedAt: done ? this.backend.clock.now().toISOString() : null, partitions: done ? MockRouteReplays.mockPartitions : 0 };
    this.replays.set(replay.id, next);
    return next;
  }
}
