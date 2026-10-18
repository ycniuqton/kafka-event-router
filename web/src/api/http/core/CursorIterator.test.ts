import { describe, expect, it } from 'vitest';
import { CursorIterator } from './CursorIterator';
import { HttpContext } from './HttpContext';
import { TestFetch } from './TestFetch.test-util';

function setup() {
  const fetch = new TestFetch();
  fetch.json(200, { items: [1, 2], next_cursor: 'c1' }).json(200, { items: [3, 4], next_cursor: 'c2' }).json(200, { items: [5], next_cursor: null });
  return { fetch, client: new HttpContext({ apiBaseUrl: '/api/v1', fetch: fetch.fn }).api };
}

describe('CursorIterator', () => {
  it('iterates every page following next_cursor', async () => {
    const { fetch, client } = setup();
    const pages: number[][] = [];
    for await (const items of new CursorIterator<number>(client, '/routes', { limit: 2, query: { status: 'running' } })) pages.push(items);
    expect(pages).toEqual([[1, 2], [3, 4], [5]]);
    expect(fetch.calls.map((c) => c.url)).toEqual([
      '/api/v1/routes?status=running&limit=2',
      '/api/v1/routes?status=running&limit=2&cursor=c1',
      '/api/v1/routes?status=running&limit=2&cursor=c2',
    ]);
  });

  it('collects up to max items and reports done', async () => {
    const { fetch, client } = setup();
    const it = new CursorIterator<number>(client, '/routes', { limit: 2 });
    expect(await it.all(3)).toEqual([1, 2, 3]);
    expect(it.done).toBe(false);
    expect(fetch.calls).toHaveLength(2);
    expect(await it.nextPage()).toEqual([5]);
    expect(it.done).toBe(true);
    expect(await it.nextPage()).toEqual([]);
  });
});
