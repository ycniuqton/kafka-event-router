import { TailCloseCode, TailFrameType, type TailClientOp, type TailFrame } from '../../../domain';
import { LiveTailService, type TailChannel, type TailHandlers } from '../../contracts';
import { ApiRoot } from '../core/HttpContext';
import { WsProtocol, WsUrl, type SocketFactory, type SocketLike } from '../core/WsClient';

/** Path of the tail socket under the API root. */
export const LiveTailEndpoint = {
  tail: (routeId: string) => `/routes/${encodeURIComponent(routeId)}/tail`,
} as const;

/** Parses server frames; anything else is ignored. */
export class TailFrameParser {
  private static readonly types = new Set<string>(Object.values(TailFrameType));

  static parse(data: unknown): TailFrame | null {
    if (typeof data !== 'string') return null;
    try {
      const value = JSON.parse(data) as TailFrame;
      return TailFrameParser.types.has(value?.type) ? value : null;
    } catch {
      return null;
    }
  }
}

/** One tail socket. */
class SocketTailChannel implements TailChannel {
  private readonly socket: SocketLike;

  constructor(socket: SocketLike, handlers: TailHandlers) {
    this.socket = socket;
    socket.onopen = () => handlers.onOpen?.();
    socket.onmessage = (ev) => {
      const frame = TailFrameParser.parse(ev.data);
      if (frame) handlers.onFrame(frame);
    };
    socket.onclose = (ev) => handlers.onClose(ev.code);
  }

  send(op: TailClientOp): boolean {
    if (this.socket.readyState !== WsProtocol.OpenReadyState) return false;
    this.socket.send(JSON.stringify(op));
    return true;
  }

  close(): void {
    this.socket.onclose = null;
    this.socket.close(TailCloseCode.Normal);
  }
}

/** `services.liveTail` over `WS /api/v1/routes/{id}/tail` (session cookie authenticates the upgrade). */
export class HttpLiveTailService extends LiveTailService {
  private readonly socketFactory: SocketFactory;
  private readonly urlOf: (path: string) => string;

  constructor(socketFactory: SocketFactory = (url) => new WebSocket(url), urlOf: (path: string) => string = (path) => WsUrl.from(path)) {
    super();
    this.socketFactory = socketFactory;
    this.urlOf = urlOf;
  }

  open(routeId: string, handlers: TailHandlers): TailChannel {
    return new SocketTailChannel(this.socketFactory(this.urlOf(`${ApiRoot.api()}${LiveTailEndpoint.tail(routeId)}`)), handlers);
  }
}
