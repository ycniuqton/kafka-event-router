import type { HttpMethod } from './HttpConstants';

export type QueryValue = string | number | boolean | null | undefined;
export type Query = Readonly<Record<string, QueryValue>>;

/** One API call. `ifMatch` carries the entity ETag on versioned writes; `silent` skips global handlers. */
export type HttpRequest = {
  method: HttpMethod;
  path: string;
  query?: Query;
  body?: unknown;
  ifMatch?: string | null;
  headers?: Readonly<Record<string, string>>;
  signal?: AbortSignal;
  silent?: boolean;
};

/** Per-call options of the convenience methods. */
export type RequestOptions = Omit<HttpRequest, 'method' | 'path' | 'body'>;

/** A successful response: parsed body plus the headers features need (ETag, request id). */
export type HttpResponse<T> = {
  data: T;
  status: number;
  etag: string | null;
  requestId: string | null;
  headers: Headers;
};
