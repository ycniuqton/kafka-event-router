import type { ApiSchemas } from '../../generated';
import { JsonPointer } from './JsonPointer';
import { ProblemCatalog, ProblemKind } from './ProblemKind';

export type Problem = ApiSchemas['Problem'];
export type FieldError = ApiSchemas['FieldError'];

/** How a request failed: no response at all, an RFC 7807 problem, or an unreadable response. */
export const ApiErrorKind = { Network: 'network', Problem: 'problem', Parse: 'parse' } as const;
export type ApiErrorKind = (typeof ApiErrorKind)[keyof typeof ApiErrorKind];

/** Messages shown when the server gives no detail. */
export const ApiErrorText = {
  Network: "Can't reach the server. Check your connection and retry.",
  Parse: 'The server sent a response we could not read.',
  Conflict: 'Someone else changed this. Reload to see their version.',
} as const;

/** A failed API call. Forms use `fieldErrors()` to highlight fields by JSON Pointer. */
export class ApiError extends Error {
  static readonly networkStatus = 0;
  readonly kind: ApiErrorKind;
  readonly status: number;
  readonly problemKind: ProblemKind | null;
  readonly problem: Problem | null;

  constructor(kind: ApiErrorKind, status: number, problem: Problem | null, message: string) {
    super(message);
    this.name = 'ApiError';
    this.kind = kind;
    this.status = status;
    this.problem = problem;
    this.problemKind = problem ? ProblemCatalog.kindOf(problem.type, status) : null;
  }

  static fromProblem(problem: Problem): ApiError {
    return new ApiError(ApiErrorKind.Problem, problem.status, problem, problem.detail ?? problem.title);
  }

  static network(cause: unknown): ApiError {
    const error = new ApiError(ApiErrorKind.Network, ApiError.networkStatus, null, ApiErrorText.Network);
    error.cause = cause;
    return error;
  }

  static parse(status: number, cause?: unknown): ApiError {
    const error = new ApiError(ApiErrorKind.Parse, status, null, ApiErrorText.Parse);
    error.cause = cause;
    return error;
  }

  get type(): string | undefined {
    return this.problem?.type;
  }

  get detail(): string | undefined {
    return this.problem?.detail;
  }

  /** JSON Pointer of the first offending field. */
  get path(): string | undefined {
    return this.problem?.path;
  }

  get errors(): FieldError[] {
    return this.problem?.errors ?? [];
  }

  get requestId(): string | undefined {
    return this.problem?.request_id;
  }

  /** Machine-readable reason (`problem.code`, e.g. `last_admin`, `login_locked`). */
  get code(): string | undefined {
    return this.problem?.code;
  }

  /** Seconds to wait before retrying (429/503: body `retry_after` or the `Retry-After` header). */
  get retryAfter(): number | undefined {
    return this.problem?.retry_after;
  }

  get currentEtag(): string | undefined {
    return this.problem?.current_etag;
  }

  is(kind: ProblemKind): boolean {
    return this.problemKind === kind;
  }

  /** `{JSON Pointer → message}`; the first message per path wins; a bare `path` uses `detail`. */
  fieldErrors(): Record<string, string> {
    const out: Record<string, string> = {};
    for (const e of this.errors) out[e.path] ??= e.message;
    if (this.path !== undefined && this.path !== JsonPointer.root) out[this.path] ??= this.message;
    return out;
  }
}
