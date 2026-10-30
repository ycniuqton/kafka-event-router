import { PushEventType, PushState, type PushEvent } from '../../../domain';
import { PushEventsService } from '../../contracts';
import { WsClient, WsMessageHandler, WsState, WsUrl, type WsMessage } from '../core/WsClient';
import { ApiRoot } from '../core/HttpContext';

/** Path of the push socket under the API root. */
export const PushEndpoint = { events: '/events' } as const;

/** Forwards every frame type to the service's listeners. */
class PushFrameHandler extends WsMessageHandler {
  readonly type: string;
  private readonly forward: (event: PushEvent) => void;

  constructor(type: string, forward: (event: PushEvent) => void) {
    super();
    this.type = type;
    this.forward = forward;
  }

  handle(message: WsMessage): void {
    this.forward(message as unknown as PushEvent);
  }
}

/** `services.push` over `WS /api/v1/events` (session cookie, reconnect with backoff while subscribed). */
export class HttpPushEventsService extends PushEventsService {
  private readonly client: WsClient;
  private readonly listeners = new Set<(event: PushEvent) => void>();

  constructor(client: WsClient = new WsClient({ url: () => WsUrl.from(`${ApiRoot.api()}${PushEndpoint.events}`) })) {
    super();
    this.client = client;
    Object.values(PushEventType).forEach((type) => this.client.on(new PushFrameHandler(type, (e) => this.listeners.forEach((l) => l(e)))));
  }

  subscribe(listener: (event: PushEvent) => void): () => void {
    this.listeners.add(listener);
    if (this.listeners.size === 1) this.client.connect();
    return () => {
      this.listeners.delete(listener);
      if (this.listeners.size === 0) this.client.close();
    };
  }

  onState(listener: (state: PushState) => void): () => void {
    return this.client.onState((s) => listener(PushStateMapper.of(s)));
  }

  state(): PushState {
    return PushStateMapper.of(this.client.state);
  }
}

/** Socket state → push state shown by the dashboard. */
export class PushStateMapper {
  static of(state: WsState): PushState {
    if (state === WsState.Open) return PushState.Live;
    return state === WsState.Connecting ? PushState.Connecting : PushState.Offline;
  }
}
