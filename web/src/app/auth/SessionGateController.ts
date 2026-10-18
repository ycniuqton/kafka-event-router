import { ApiError, ProblemKind, type SessionService, type SessionUser } from '../../api/http/core';
import { AuthPaths } from './AuthPaths';

/** What the gate shows: wait, the app, a redirect to login, or a retryable error. */
export const GateStatus = { Checking: 'checking', Authenticated: 'authenticated', Redirect: 'redirect', Failed: 'failed' } as const;
export type GateStatus = (typeof GateStatus)[keyof typeof GateStatus];

export type GateOutcome =
  | { status: typeof GateStatus.Checking }
  | { status: typeof GateStatus.Authenticated; user: SessionUser }
  | { status: typeof GateStatus.Redirect; to: string }
  | { status: typeof GateStatus.Failed; message: string };

/** Decides the gate outcome from `/auth/me`: 200 → app, 401 → login with `return_to`, else error. */
export class SessionGateController {
  private readonly session: SessionService;

  constructor(session: SessionService) {
    this.session = session;
  }

  async resolve(currentPath: string): Promise<GateOutcome> {
    try {
      return { status: GateStatus.Authenticated, user: await this.session.me() };
    } catch (error) {
      return SessionGateController.fromError(error, currentPath);
    }
  }

  static fromError(error: unknown, currentPath: string): GateOutcome {
    if (error instanceof ApiError && error.is(ProblemKind.Unauthenticated)) {
      return { status: GateStatus.Redirect, to: AuthPaths.loginUrl(currentPath) };
    }
    return { status: GateStatus.Failed, message: error instanceof Error ? error.message : String(error) };
  }
}
