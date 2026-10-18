import { describe, expect, it, vi } from 'vitest';
import { ApiError, ApiErrorBus, ApiErrorText, HttpMethod, ProblemCatalog, ProblemKind, SessionService } from '../../api/http/core';
import { AuthPaths } from './AuthPaths';
import { GlobalApiErrorHandlers } from './GlobalApiErrorHandlers';
import { MockSessionService } from './MockSessionService';
import { GateStatus, SessionGateController } from './SessionGateController';

const problem = (kind: ProblemKind, status: number) => ApiError.fromProblem({ type: ProblemCatalog.typeOf(kind), title: kind, status });

class FailingSession extends MockSessionService {
  private readonly error: unknown;
  constructor(error: unknown) {
    super();
    this.error = error;
  }
  me(): ReturnType<SessionService['me']> {
    return Promise.reject(this.error);
  }
}

describe('SessionGateController', () => {
  it('lets a signed-in user through', async () => {
    const outcome = await new SessionGateController(new MockSessionService()).resolve('/routes');
    expect(outcome).toEqual({ status: GateStatus.Authenticated, user: MockSessionService.user });
  });

  it('redirects a 401 to /login with return_to', async () => {
    const outcome = await new SessionGateController(new FailingSession(problem(ProblemKind.Unauthenticated, 401))).resolve('/routes?q=a');
    expect(outcome).toEqual({ status: GateStatus.Redirect, to: '/login?return_to=%2Froutes%3Fq%3Da' });
  });

  it('reports other failures as retryable errors', async () => {
    const outcome = await new SessionGateController(new FailingSession(ApiError.network(new TypeError()))).resolve('/');
    expect(outcome).toEqual({ status: GateStatus.Failed, message: ApiErrorText.Network });
  });
});

describe('AuthPaths.safeReturnTo', () => {
  it('accepts only relative in-app paths', () => {
    expect(AuthPaths.safeReturnTo('/routes/r1?tab=dead')).toBe('/routes/r1?tab=dead');
    expect(AuthPaths.safeReturnTo('//evil.example')).toBe('/');
    expect(AuthPaths.safeReturnTo('https://evil.example')).toBe('/');
    expect(AuthPaths.safeReturnTo('/login?return_to=/x')).toBe('/');
    expect(AuthPaths.safeReturnTo(null)).toBe('/');
  });
});

describe('GlobalApiErrorHandlers', () => {
  it('redirects on 401 and toasts on 412 and network errors', () => {
    const bus = new ApiErrorBus();
    const toLogin = vi.fn();
    const toast = vi.fn();
    const uninstall = GlobalApiErrorHandlers.install(bus, { toLogin, toast });
    const request = { method: HttpMethod.Get, path: '/routes' };
    bus.publish(problem(ProblemKind.Unauthenticated, 401), request);
    bus.publish(problem(ProblemKind.PreconditionFailed, 412), request);
    bus.publish(ApiError.network(new TypeError()), request);
    bus.publish(problem(ProblemKind.Validation, 422), request);
    expect(toLogin).toHaveBeenCalledTimes(1);
    expect(toast.mock.calls).toEqual([[ApiErrorText.Conflict], [ApiErrorText.Network]]);
    uninstall();
    bus.publish(problem(ProblemKind.Unauthenticated, 401), request);
    expect(toLogin).toHaveBeenCalledTimes(1);
  });
});
