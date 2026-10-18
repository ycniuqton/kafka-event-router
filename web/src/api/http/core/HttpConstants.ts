/** HTTP vocabulary shared by the REST client: methods, headers, media types, statuses and API roots. */
export const HttpMethod = { Get: 'GET', Post: 'POST', Put: 'PUT', Patch: 'PATCH', Delete: 'DELETE' } as const;
export type HttpMethod = (typeof HttpMethod)[keyof typeof HttpMethod];

export const HttpHeader = {
  Accept: 'Accept',
  ContentType: 'Content-Type',
  CsrfToken: 'X-CSRF-Token',
  RequestId: 'X-Request-ID',
  IfMatch: 'If-Match',
  ETag: 'ETag',
  RetryAfter: 'Retry-After',
} as const;

export const MediaType = { Json: 'application/json', Problem: 'application/problem+json' } as const;

export const HttpStatus = {
  NoContent: 204,
  BadRequest: 400,
  Unauthorized: 401,
  Forbidden: 403,
  NotFound: 404,
  Conflict: 409,
  PreconditionFailed: 412,
  UnprocessableEntity: 422,
  PreconditionRequired: 428,
  TooManyRequests: 429,
  InternalServerError: 500,
  ServiceUnavailable: 503,
} as const;

/** Methods that change state: they carry the CSRF token (double-submit cookie). */
export class MutatingMethods {
  private static readonly set: ReadonlySet<string> = new Set([HttpMethod.Post, HttpMethod.Put, HttpMethod.Patch, HttpMethod.Delete]);

  static has(method: string): boolean {
    return MutatingMethods.set.has(method.toUpperCase());
  }
}

/** Credentials mode: session cookies travel with every request. */
export const FetchCredentials = { Include: 'include' } as const;
