import { describe, expect, it } from 'vitest';
import { ApiError } from './ApiError';
import { FieldErrorMapper } from './FieldErrorMapper';
import { JsonPointer } from './JsonPointer';
import { ProblemCatalog, ProblemKind } from './ProblemKind';

describe('JsonPointer', () => {
  it('parses and formats RFC 6901 pointers with escapes', () => {
    expect(JsonPointer.parse('/a~1b/m~0n/0')).toEqual(['a/b', 'm~n', '0']);
    expect(JsonPointer.format(['a/b', 'm~n', 0])).toBe('/a~1b/m~0n/0');
    expect(JsonPointer.parse('')).toEqual([]);
    expect(JsonPointer.toFieldName('/config/retries')).toBe('config.retries');
  });
});

describe('ApiError', () => {
  it('derives the kind from the type URI, else from the status', () => {
    expect(ProblemCatalog.kindOf(ProblemCatalog.typeOf(ProblemKind.Conflict), 409)).toBe(ProblemKind.Conflict);
    expect(ProblemCatalog.kindOf('about:blank', 401)).toBe(ProblemKind.Unauthenticated);
    expect(ProblemCatalog.kindOf(undefined, 418)).toBe(ProblemKind.Internal);
  });

  it('uses the problem path and detail when there is no errors[]', () => {
    const error = ApiError.fromProblem({ type: ProblemCatalog.typeOf(ProblemKind.MalformedBody), title: 'Bad Request', status: 400, detail: 'Bad cursor.', path: '/cursor' });
    expect(error.fieldErrors()).toEqual({ '/cursor': 'Bad cursor.' });
    expect(error.message).toBe('Bad cursor.');
  });

  it('exposes current_etag of a 412', () => {
    const error = ApiError.fromProblem({ type: ProblemCatalog.typeOf(ProblemKind.PreconditionFailed), title: 'Precondition Failed', status: 412, current_etag: '"9"' });
    expect(error.currentEtag).toBe('"9"');
  });
});

describe('FieldErrorMapper', () => {
  it('maps pointers to dotted field names, honouring the longest alias prefix', () => {
    const error = ApiError.fromProblem({
      type: ProblemCatalog.typeOf(ProblemKind.Validation),
      title: 'Unprocessable Entity',
      status: 422,
      errors: [
        { path: '/bootstrapServers/0', code: 'host_port', message: 'Use host:port.' },
        { path: '/security/sasl/password', code: 'required', message: 'Required.' },
        { path: '/name', code: 'duplicate', message: 'Taken.' },
      ],
    });
    const mapper = new FieldErrorMapper({ '/bootstrapServers': 'servers', '/security': 'security', '/security/sasl': 'sasl' });
    expect(mapper.map(error)).toEqual({ servers: 'Use host:port.', sasl: 'Required.', name: 'Taken.' });
  });
});
