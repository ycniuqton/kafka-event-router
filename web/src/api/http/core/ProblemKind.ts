import { HttpStatus } from './HttpConstants';

/** Problem categories (mirror of the backend `problem.Kind` catalog; RFC 7807 `type` suffixes). */
export const ProblemKind = {
  Validation: 'validation',
  MalformedBody: 'malformed-body',
  Unauthenticated: 'unauthenticated',
  Forbidden: 'forbidden',
  NotFound: 'not-found',
  MethodNotAllowed: 'method-not-allowed',
  Conflict: 'conflict',
  PreconditionFailed: 'precondition-failed',
  PayloadTooLarge: 'payload-too-large',
  PreconditionRequired: 'precondition-required',
  RateLimited: 'rate-limited',
  Unavailable: 'unavailable',
  Internal: 'internal',
} as const;
export type ProblemKind = (typeof ProblemKind)[keyof typeof ProblemKind];

/** Resolves a problem kind from its `type` URI, falling back to the HTTP status for `about:blank`. */
export class ProblemCatalog {
  static readonly typeBase = 'https://eventrouter.dev/problems/';
  private static readonly known: ReadonlySet<string> = new Set(Object.values(ProblemKind));
  private static readonly byStatus: ReadonlyMap<number, ProblemKind> = new Map<number, ProblemKind>([
    [HttpStatus.BadRequest, ProblemKind.MalformedBody],
    [HttpStatus.Unauthorized, ProblemKind.Unauthenticated],
    [HttpStatus.Forbidden, ProblemKind.Forbidden],
    [HttpStatus.NotFound, ProblemKind.NotFound],
    [HttpStatus.Conflict, ProblemKind.Conflict],
    [HttpStatus.PreconditionFailed, ProblemKind.PreconditionFailed],
    [HttpStatus.UnprocessableEntity, ProblemKind.Validation],
    [HttpStatus.PreconditionRequired, ProblemKind.PreconditionRequired],
    [HttpStatus.TooManyRequests, ProblemKind.RateLimited],
    [HttpStatus.ServiceUnavailable, ProblemKind.Unavailable],
  ]);

  static kindOf(type: string | undefined, status: number): ProblemKind {
    const suffix = type?.startsWith(ProblemCatalog.typeBase) ? type.slice(ProblemCatalog.typeBase.length) : '';
    if (ProblemCatalog.known.has(suffix)) return suffix as ProblemKind;
    return ProblemCatalog.byStatus.get(status) ?? ProblemKind.Internal;
  }

  static typeOf(kind: ProblemKind): string {
    return ProblemCatalog.typeBase + kind;
  }
}
