import { vi } from 'vitest';
import { ApiError } from './ApiError';
import type { FetchFn } from './HttpClient';

/** A recorded fetch call. */
export type FetchCall = { url: string; init: RequestInit };

/** Scripted fetch for tests: returns queued responses and records calls. */
export class TestFetch {
  readonly calls: FetchCall[] = [];
  private readonly queue: (() => Response)[] = [];
  readonly fn: FetchFn = vi.fn((url: string, init?: RequestInit) => {
    this.calls.push({ url, init: init ?? {} });
    const next = this.queue.shift();
    return next ? Promise.resolve(next()) : Promise.reject(new TypeError('Failed to fetch'));
  });

  json(status: number, body: unknown, headers: Record<string, string> = {}): this {
    this.queue.push(() => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json', ...headers } }));
    return this;
  }

  problem(status: number, body: unknown): this {
    this.queue.push(() => new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/problem+json' } }));
    return this;
  }

  empty(status: number): this {
    this.queue.push(() => new Response(null, { status }));
    return this;
  }

  /** Awaits a call that must fail and returns its ApiError. */
  static async failure(call: Promise<unknown>): Promise<ApiError> {
    try {
      await call;
    } catch (error) {
      if (error instanceof ApiError) return error;
      throw error;
    }
    throw new Error('expected the call to fail');
  }

  header(index: number, name: string): string | null {
    return new Headers(this.calls[index].init.headers).get(name);
  }
}
