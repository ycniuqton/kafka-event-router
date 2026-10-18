import { ApiError, type Problem } from './ApiError';
import { FetchCredentials, HttpHeader, HttpStatus, MediaType } from './HttpConstants';
import type { HttpRequest, HttpResponse, Query } from './HttpRequest';

/** Builds request URLs: base + path + query (null/undefined query values are dropped). */
export class UrlBuilder {
  static build(baseUrl: string, path: string, query?: Query): string {
    const params = new URLSearchParams();
    Object.entries(query ?? {}).forEach(([key, value]) => {
      if (value !== undefined && value !== null) params.set(key, String(value));
    });
    const qs = params.toString();
    return baseUrl + path + (qs ? `?${qs}` : '');
  }
}

/** A non-JSON request body (e.g. a CSV upload) sent as-is with its media type. */
export class RawBody {
  readonly text: string;
  readonly mediaType: string;

  constructor(text: string, mediaType: string) {
    this.text = text;
    this.mediaType = mediaType;
  }
}

/** Builds the `fetch` init: JSON (or raw) body, Accept, CSRF token on writes, If-Match when given. */
export class RequestInitBuilder {
  static build(req: HttpRequest, csrfToken: string | null): RequestInit {
    const headers = new Headers(req.headers);
    const raw = req.body instanceof RawBody ? req.body : null;
    headers.set(HttpHeader.Accept, `${MediaType.Json}, ${MediaType.Problem}`);
    if (req.body !== undefined) headers.set(HttpHeader.ContentType, raw ? raw.mediaType : MediaType.Json);
    if (csrfToken) headers.set(HttpHeader.CsrfToken, csrfToken);
    if (req.ifMatch) headers.set(HttpHeader.IfMatch, req.ifMatch);
    return {
      method: req.method,
      headers,
      body: raw ? raw.text : req.body === undefined ? undefined : JSON.stringify(req.body),
      credentials: FetchCredentials.Include,
      signal: req.signal,
    };
  }
}

/** Reads a successful response; 204 and empty bodies resolve to `undefined`. */
export class ResponseReader {
  static async read<T>(response: Response): Promise<HttpResponse<T>> {
    const text = response.status === HttpStatus.NoContent ? '' : await response.text();
    let data: T;
    try {
      data = (text ? JSON.parse(text) : undefined) as T;
    } catch (cause) {
      throw ApiError.parse(response.status, cause);
    }
    return {
      data,
      status: response.status,
      etag: response.headers.get(HttpHeader.ETag),
      requestId: response.headers.get(HttpHeader.RequestId),
      headers: response.headers,
    };
  }
}

/** Turns a failed response into an `ApiError`, synthesising a problem when the body is not one. */
export class ProblemReader {
  static readonly blankType = 'about:blank';

  static async read(response: Response): Promise<ApiError> {
    const parsed = await ProblemReader.parse(response);
    const problem: Problem = ProblemReader.isProblem(parsed)
      ? parsed
      : { type: ProblemReader.blankType, title: response.statusText, status: response.status };
    problem.request_id ??= response.headers.get(HttpHeader.RequestId) ?? undefined;
    return ApiError.fromProblem(problem);
  }

  private static async parse(response: Response): Promise<unknown> {
    try {
      return await response.json();
    } catch {
      return null;
    }
  }

  private static isProblem(value: unknown): value is Problem {
    return typeof value === 'object' && value !== null && typeof (value as Problem).status === 'number';
  }
}
