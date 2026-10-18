import { describe, expect, it } from 'vitest';
import { ApiError, ProblemCatalog, ProblemKind } from '../api/http/core';
import { ForbiddenText, ForbiddenToast } from './ForbiddenToast';

function problem(status: number, detail?: string): ApiError {
  const kind = status === 403 ? ProblemKind.Forbidden : ProblemKind.Conflict;
  return ApiError.fromProblem({ type: ProblemCatalog.typeOf(kind), title: 't', status, detail });
}

describe('ForbiddenToast', () => {
  it('toasts the problem detail on 403 and reloads access', () => {
    const toasts: string[] = [];
    let reloads = 0;
    const listener = new ForbiddenToast((m) => toasts.push(m), () => reloads++);
    listener.handle(problem(403, 'You need route.write on team Logistics.'));
    listener.handle(problem(403));
    listener.handle(problem(409, 'conflict'));
    expect(toasts).toEqual(['You need route.write on team Logistics.', ForbiddenText.fallback]);
    expect(reloads).toBe(2);
  });
});
