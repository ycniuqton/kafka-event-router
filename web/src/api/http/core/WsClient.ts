/** A server push message; `type` routes it to handlers (e.g. `route.status`, `alert.fired`). */
export type WsMessage = { type: string } & Record<string, unknown>;

/** Base for class-based push handlers; one handler per message `type`. */
export abstract class WsMessageHandler<M extends WsMessage = WsMessage> {
  abstract readonly type: string;
  abstract handle(message: M): void;
}

/** The subset of `WebSocket` the client needs (lets tests inject a fake). */
export type SocketLike = {
  readyState: number;
  onopen: ((ev: Event) => void) | null;
  onclose: ((ev: CloseEvent) => void) | null;
  onmessage: ((ev: MessageEvent) => void) | null;
  send(data: string): void;
  close(code?: number): void;
};
export type SocketFactory = (url: string) => SocketLike;

export const WsState = { Idle: 'idle', Connecting: 'connecting', Open: 'open', Closed: 'closed' } as const;
export type WsState = (typeof WsState)[keyof typeof WsState];

/** WebSocket protocol values used by the client. */
export const WsProtocol = { Secure: 'wss:', Plain: 'ws:', HttpsScheme: 'https:', OpenReadyState: 1, NormalClose: 1000 } as const;

/** Exponential backoff between reconnect attempts, capped. */
export class ReconnectPolicy {
  static readonly initialMs = 500;
  static readonly maxMs = 30_000;
  static readonly factor = 2;

  delay(attempt: number): number {
    return Math.min(ReconnectPolicy.maxMs, ReconnectPolicy.initialMs * ReconnectPolicy.factor ** attempt);
  }
}

/** Builds an absolute ws(s):// URL for a same-origin path. */
export class WsUrl {
  static from(path: string, location: Pick<Location, 'protocol' | 'host'> = window.location): string {
    const scheme = location.protocol === WsProtocol.HttpsScheme ? WsProtocol.Secure : WsProtocol.Plain;
    return `${scheme}//${location.host}${path}`;
  }
}

/** Socket address, or a resolver called on each connect (keeps `window` out of service construction). */
export type WsAddress = string | (() => string);

export type WsClientOptions = { url: WsAddress; socketFactory?: SocketFactory; reconnect?: ReconnectPolicy };

/**
 * Base WebSocket client: JSON messages dispatched by `type` to handlers, automatic reconnect
 * with backoff until `close()`. Session cookies authenticate the upgrade (same origin).
 */
export class WsClient {
  private readonly url: WsAddress;
  private readonly socketFactory: SocketFactory;
  private readonly reconnect: ReconnectPolicy;
  private readonly handlers = new Map<string, Set<WsMessageHandler>>();
  private readonly stateListeners = new Set<(state: WsState) => void>();
  private socket: SocketLike | null = null;
  private attempt = 0;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private wanted = false;
  state: WsState = WsState.Idle;

  constructor(options: WsClientOptions) {
    this.url = options.url;
    this.socketFactory = options.socketFactory ?? ((url) => new WebSocket(url));
    this.reconnect = options.reconnect ?? new ReconnectPolicy();
  }

  connect(): void {
    this.wanted = true;
    if (this.socket) return;
    this.setState(WsState.Connecting);
    const socket = this.socketFactory(typeof this.url === 'function' ? this.url() : this.url);
    socket.onopen = () => this.opened();
    socket.onmessage = (ev) => this.dispatch(ev.data);
    socket.onclose = () => this.closed();
    this.socket = socket;
  }

  close(): void {
    this.wanted = false;
    clearTimeout(this.timer);
    this.socket?.close(WsProtocol.NormalClose);
    this.socket = null;
    this.setState(WsState.Closed);
  }

  /** Sends a JSON message; returns false when the socket is not open. */
  send(message: WsMessage): boolean {
    if (!this.socket || this.socket.readyState !== WsProtocol.OpenReadyState) return false;
    this.socket.send(JSON.stringify(message));
    return true;
  }

  /** Registers a handler; returns the unsubscribe function. */
  on(handler: WsMessageHandler): () => void {
    const set = this.handlers.get(handler.type) ?? new Set<WsMessageHandler>();
    set.add(handler);
    this.handlers.set(handler.type, set);
    return () => set.delete(handler);
  }

  onState(listener: (state: WsState) => void): () => void {
    this.stateListeners.add(listener);
    return () => this.stateListeners.delete(listener);
  }

  private opened(): void {
    this.attempt = 0;
    this.setState(WsState.Open);
  }

  private closed(): void {
    this.socket = null;
    this.setState(WsState.Closed);
    if (!this.wanted) return;
    this.timer = setTimeout(() => this.connect(), this.reconnect.delay(this.attempt));
    this.attempt += 1;
  }

  private dispatch(data: unknown): void {
    const message = WsClient.parse(data);
    if (message) this.handlers.get(message.type)?.forEach((h) => h.handle(message));
  }

  private setState(state: WsState): void {
    this.state = state;
    this.stateListeners.forEach((l) => l(state));
  }

  private static parse(data: unknown): WsMessage | null {
    if (typeof data !== 'string') return null;
    try {
      const value = JSON.parse(data) as WsMessage;
      return typeof value?.type === 'string' ? value : null;
    } catch {
      return null;
    }
  }
}
