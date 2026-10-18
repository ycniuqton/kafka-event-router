import type { ApiSchemas } from '../../generated';
import type { HttpClient } from './HttpClient';
import type { Query, RequestOptions } from './HttpRequest';

/** A cursor page `{items, next_cursor}`; `next_cursor` is null on the last page. */
export type CursorPage<T> = ApiSchemas['PageMeta'] & { items: T[] };

/** Page sizes accepted by list endpoints (server clamps above `Max`). */
export const PageLimit = { Default: 50, Max: 200 } as const;

/** Query keys of cursor pagination. */
export const PageParam = { Limit: 'limit', Cursor: 'cursor' } as const;

export type CursorIteratorOptions = { limit?: number; query?: Query; cursor?: string | null; request?: RequestOptions };

/**
 * Walks a cursor-paginated list. Use `for await (const items of it)` for every page,
 * `nextPage()` for "load more" UIs, or `all(max)` to collect.
 */
export class CursorIterator<T> implements AsyncIterable<T[]> {
  private readonly client: HttpClient;
  private readonly path: string;
  private readonly options: CursorIteratorOptions;
  private cursor: string | null;
  private exhausted = false;

  constructor(client: HttpClient, path: string, options: CursorIteratorOptions = {}) {
    this.client = client;
    this.path = path;
    this.options = options;
    this.cursor = options.cursor ?? null;
  }

  get done(): boolean {
    return this.exhausted;
  }

  /** Fetches the next page; resolves to an empty list once the last page was read. */
  async nextPage(): Promise<T[]> {
    if (this.exhausted) return [];
    const query = { ...this.options.query, [PageParam.Limit]: this.options.limit ?? PageLimit.Default, [PageParam.Cursor]: this.cursor };
    const page = await this.client.get<CursorPage<T>>(this.path, query, this.options.request);
    this.cursor = page.next_cursor;
    this.exhausted = !page.next_cursor;
    return page.items;
  }

  async *[Symbol.asyncIterator](): AsyncIterator<T[]> {
    while (!this.exhausted) yield await this.nextPage();
  }

  /** Collects items across pages, stopping after `max` items. */
  async all(max: number = Number.POSITIVE_INFINITY): Promise<T[]> {
    const out: T[] = [];
    while (!this.exhausted && out.length < max) out.push(...(await this.nextPage()));
    return out.slice(0, max);
  }
}
