import { describe, expect, it } from 'vitest';
import { TailFrameType, TailOp, TailState, type TailFrame } from '../../../domain';
import type { SocketLike } from '../core/WsClient';
import { HttpLiveTailService, TailFrameParser } from './HttpLiveTailService';

class FakeSocket implements SocketLike {
  readyState = 1;
  onopen: ((ev: Event) => void) | null = null;
  onclose: ((ev: CloseEvent) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  sent: string[] = [];
  closedWith: number | undefined;
  send(data: string): void {
    this.sent.push(data);
  }
  close(code?: number): void {
    this.closedWith = code;
  }
}

describe('HttpLiveTailService', () => {
  it('opens the route tail socket, parses frames, reports close codes and sends ops', () => {
    const socket = new FakeSocket();
    const urls: string[] = [];
    const frames: TailFrame[] = [];
    const codes: number[] = [];
    const channel = new HttpLiveTailService((url) => (urls.push(url), socket), (path) => `ws://localhost${path}`).open('r 1', { onFrame: (f) => frames.push(f), onClose: (c) => codes.push(c) });
    expect(urls[0]).toMatch(/\/api\/v1\/routes\/r%201\/tail$/);
    socket.onmessage?.({ data: JSON.stringify({ type: TailFrameType.Status, state: TailState.Live, masked: true }) } as MessageEvent);
    socket.onmessage?.({ data: 'not json' } as MessageEvent);
    socket.onmessage?.({ data: JSON.stringify({ type: 'hello' }) } as MessageEvent);
    expect(frames).toEqual([{ type: TailFrameType.Status, state: TailState.Live, masked: true }]);
    expect(channel.send({ op: TailOp.Pause })).toBe(true);
    expect(socket.sent).toEqual(['{"op":"pause"}']);
    socket.onclose?.({ code: 4409 } as CloseEvent);
    expect(codes).toEqual([4409]);
    channel.close();
    expect(socket.closedWith).toBe(1000);
  });

  it('ignores non-string frames', () => {
    expect(TailFrameParser.parse(new ArrayBuffer(2))).toBeNull();
  });
});
