import { afterEach, describe, expect, it, vi } from 'vitest';
import { ReconnectPolicy, WsClient, WsMessageHandler, WsState, WsUrl, type SocketLike, type WsMessage } from './WsClient';

class FakeSocket implements SocketLike {
  readyState = 0;
  onopen: SocketLike['onopen'] = null;
  onclose: SocketLike['onclose'] = null;
  onmessage: SocketLike['onmessage'] = null;
  readonly sent: string[] = [];
  send(data: string): void {
    this.sent.push(data);
  }
  close(): void {
    this.readyState = 3;
  }
  open(): void {
    this.readyState = 1;
    this.onopen?.(new Event('open'));
  }
  receive(data: string): void {
    this.onmessage?.({ data } as MessageEvent);
  }
  drop(): void {
    this.readyState = 3;
    this.onclose?.({} as CloseEvent);
  }
}

class Collect extends WsMessageHandler {
  readonly type = 'route.status';
  readonly got: WsMessage[] = [];
  handle(message: WsMessage): void {
    this.got.push(message);
  }
}

afterEach(() => vi.useRealTimers());

describe('WsClient', () => {
  it('dispatches JSON messages by type and sends only when open', () => {
    const sockets: FakeSocket[] = [];
    const client = new WsClient({ url: 'ws://x/api/v1/events', socketFactory: () => sockets[sockets.push(new FakeSocket()) - 1] });
    const handler = new Collect();
    client.on(handler);
    client.connect();
    expect(client.send({ type: 'ping' })).toBe(false);
    sockets[0].open();
    expect(client.state).toBe(WsState.Open);
    expect(client.send({ type: 'ping' })).toBe(true);
    sockets[0].receive('{"type":"route.status","id":"r1"}');
    sockets[0].receive('{"type":"other"}');
    sockets[0].receive('not json');
    expect(handler.got).toEqual([{ type: 'route.status', id: 'r1' }]);
  });

  it('reconnects with backoff until closed', () => {
    vi.useFakeTimers();
    const sockets: FakeSocket[] = [];
    const client = new WsClient({ url: 'ws://x', socketFactory: () => sockets[sockets.push(new FakeSocket()) - 1] });
    client.connect();
    sockets[0].drop();
    vi.advanceTimersByTime(new ReconnectPolicy().delay(0));
    expect(sockets).toHaveLength(2);
    client.close();
    sockets[1].drop();
    vi.advanceTimersByTime(ReconnectPolicy.maxMs);
    expect(sockets).toHaveLength(2);
  });

  it('builds ws/wss URLs from the page location', () => {
    expect(WsUrl.from('/api/v1/events', { protocol: 'https:', host: 'er.example' })).toBe('wss://er.example/api/v1/events');
    expect(WsUrl.from('/e', { protocol: 'http:', host: 'localhost:5173' })).toBe('ws://localhost:5173/e');
  });
});
