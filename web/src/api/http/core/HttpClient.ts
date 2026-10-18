import { ApiError } from './ApiError';
import type { ApiErrorBus } from './ApiErrorBus';
import type { CsrfTokenStore } from './CsrfTokenStore';
import { HttpMethod, MutatingMethods } from './HttpConstants';
import { ProblemReader, RequestInitBuilder, ResponseReader, UrlBuilder } from './HttpMessages';
import type { HttpRequest, HttpResponse, Query, RequestOptions } from './HttpRequest';

export type FetchFn = (input: string, init?: RequestInit) => Promise<Response>;

export type HttpClientOptions = {
  baseUrl: string;
  csrf: CsrfTokenStore;
  errors: ApiErrorBus;
  fetch?: FetchFn;
};

/**
 * JSON-over-HTTP client for one API root: cookies, CSRF on writes, If-Match, RFC 7807 errors.
 * Every failure is thrown as an `ApiError` and published to the shared `ApiErrorBus`.
 */
export class HttpClient {
  readonly baseUrl: string;
  private readonly csrf: CsrfTokenStore;
  private readonly errors: ApiErrorBus;
  private readonly fetchFn: FetchFn;

  constructor(options: HttpClientOptions) {
    this.baseUrl = options.baseUrl;
    this.csrf = options.csrf;
    this.errors = options.errors;
    this.fetchFn = options.fetch ?? ((input, init) => globalThis.fetch(input, init));
  }

  /** Sends a request and parses the JSON body with its ETag and request id. */
  async send<T>(req: HttpRequest): Promise<HttpResponse<T>> {
    const response = await this.raw(req);
    try {
      return await ResponseReader.read<T>(response);
    } catch (error) {
      throw this.fail(req, error instanceof ApiError ? error : ApiError.parse(response.status, error));
    }
  }

  /** Sends a request and returns the unread `Response` (streams such as NDJSON test progress). */
  async raw(req: HttpRequest): Promise<Response> {
    const response = await this.dispatch(req);
    if (!response.ok) throw this.fail(req, await ProblemReader.read(response));
    return response;
  }

  async get<T>(path: string, query?: Query, options: RequestOptions = {}): Promise<T> {
    return (await this.send<T>({ ...options, method: HttpMethod.Get, path, query: query ?? options.query })).data;
  }

  async post<T>(path: string, body?: unknown, options: RequestOptions = {}): Promise<T> {
    return (await this.send<T>({ ...options, method: HttpMethod.Post, path, body })).data;
  }

  async put<T>(path: string, body?: unknown, options: RequestOptions = {}): Promise<T> {
    return (await this.send<T>({ ...options, method: HttpMethod.Put, path, body })).data;
  }

  async patch<T>(path: string, body?: unknown, options: RequestOptions = {}): Promise<T> {
    return (await this.send<T>({ ...options, method: HttpMethod.Patch, path, body })).data;
  }

  async delete<T = void>(path: string, options: RequestOptions = {}): Promise<T> {
    return (await this.send<T>({ ...options, method: HttpMethod.Delete, path })).data;
  }

  private async dispatch(req: HttpRequest): Promise<Response> {
    const csrf = MutatingMethods.has(req.method) ? this.csrf.current() : null;
    const url = UrlBuilder.build(this.baseUrl, req.path, req.query);
    try {
      return await this.fetchFn(url, RequestInitBuilder.build(req, csrf));
    } catch (cause) {
      if (req.signal?.aborted) throw cause;
      throw this.fail(req, ApiError.network(cause));
    }
  }

  private fail(req: HttpRequest, error: ApiError): ApiError {
    this.errors.publish(error, req);
    return error;
  }
}
