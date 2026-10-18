import type { LiveTailService, TailChannel } from '../../../api/contracts';
import { TailCloseCode, TailFrameType, TailOp, TailShowRules, TailState, type TailFilter, type TailFrame, type TailRecord } from '../../../domain';

/** Exponential backoff between reconnects, capped. */
export class TailBackoff {
  static readonly initialMs = 500;
  static readonly maxMs = 15_000;
  static readonly factor = 2;

  delay(attempt: number): number {
    return Math.min(TailBackoff.maxMs, TailBackoff.initialMs * TailBackoff.factor ** attempt);
  }
}

export type TailClientListener = {
  onRecord: (record: TailRecord, masked: boolean) => void;
  onState: (state: TailState, masked: boolean) => void;
};

/** Timer functions (injected by tests). */
export type TailTimers = { set: (fn: () => void, ms: number) => unknown; clear: (handle: unknown) => void };
const browserTimers: TailTimers = {
  set: (fn, ms) => setTimeout(fn, ms),
  clear: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
};

/**
 * One route's live tail connection: reconnects with backoff (except when the route is not running),
 * and re-applies the pause state and filter after every (re)connect.
 */
export class TailSocketClient {
  private channel: TailChannel | null = null;
  private attempt = 0;
  private timer: unknown;
  private stopped = false;
  private paused = false;
  private filter: TailFilter | null = null;
  private masked = false;
  private readonly service: LiveTailService;
  private readonly routeId: string;
  private readonly listener: TailClientListener;
  private readonly backoff: TailBackoff;
  private readonly timers: TailTimers;

  constructor(service: LiveTailService, routeId: string, listener: TailClientListener, backoff = new TailBackoff(), timers = browserTimers) {
    this.service = service;
    this.routeId = routeId;
    this.listener = listener;
    this.backoff = backoff;
    this.timers = timers;
  }

  start(): void {
    this.stopped = false;
    this.listener.onState(TailState.Connecting, this.masked);
    this.channel = this.service.open(this.routeId, {
      onOpen: () => this.opened(),
      onFrame: (frame) => this.frame(frame),
      onClose: (code) => this.closed(code),
    });
  }

  stop(): void {
    this.stopped = true;
    this.timers.clear(this.timer);
    this.channel?.close();
    this.channel = null;
  }

  setPaused(paused: boolean): void {
    this.paused = paused;
    this.channel?.send({ op: paused ? TailOp.Pause : TailOp.Resume });
  }

  setFilter(filter: TailFilter): void {
    this.filter = filter;
    this.channel?.send(TailShowRules.op(filter));
  }

  private opened(): void {
    this.attempt = 0;
    if (this.filter) this.channel?.send(TailShowRules.op(this.filter));
    if (this.paused) this.channel?.send({ op: TailOp.Pause });
  }

  private frame(frame: TailFrame): void {
    this.masked = frame.masked;
    if (frame.type === TailFrameType.Record) this.listener.onRecord(frame.record, frame.masked);
    else this.listener.onState(frame.state, frame.masked);
  }

  private closed(code: number): void {
    this.channel = null;
    if (this.stopped) return;
    if (code === TailCloseCode.NotRunning) {
      this.listener.onState(TailState.NotRunning, this.masked);
      return;
    }
    this.listener.onState(code === TailCloseCode.Unavailable ? TailState.Unavailable : TailState.Reconnecting, this.masked);
    this.timer = this.timers.set(() => this.start(), this.backoff.delay(this.attempt));
    this.attempt += 1;
  }
}
