import { ApiError, ProblemKind, type SessionService, type SessionUser } from '../../api/http/core';
import { AuthPaths } from './AuthPaths';

/** What the gate shows: wait, the app, the forced password change, a redirect to login, or a retryable error. */
export const GateStatus = {
  Checking: 'checking',
  Authenticated: 'authenticated',
  PasswordChange: 'password_change',
  Redirect: 'redirect',
  Failed: 'failed',
} as const;
export type GateStatus = (typeof GateStatus)[keyof typeof GateStatus];

export type GateOutcome =
  | { status: typeof GateStatus.Checking }
  | { status: typeof GateStatus.Authenticated; user: SessionUser }
  | { status: typeof GateStatus.PasswordChange; user: SessionUser }
  | { status: typeof GateStatus.Redirect; to: string }
  | { status: typeof GateStatus.Failed; message: string };

/** Decides the gate outcome from `/auth/me`: 200 → app (or the forced password change), 401 → login with `return_to`, else error. */
export class SessionGateController {
  private readonly session: SessionService;

  constructor(session: SessionService) {
    this.session = session;
  }

  async resolve(currentPath: string): Promise<GateOutcome> {
    try {
      return SessionGateController.fromUser(await this.session.me());
    } catch (error) {
      return SessionGateController.fromError(error, currentPath);
    }
  }

  /** `must_change_password` (absent = false) blocks the app until the password is changed. */
  static fromUser(user: SessionUser): GateOutcome {
    if (user.must_change_password) return { status: GateStatus.PasswordChange, user };
    return { status: GateStatus.Authenticated, user };
  }

  static fromError(error: unknown, currentPath: string): GateOutcome {
    if (error instanceof ApiError && error.is(ProblemKind.Unauthenticated)) {
      return { status: GateStatus.Redirect, to: AuthPaths.loginUrl(currentPath) };
    }
    return { status: GateStatus.Failed, message: error instanceof Error ? error.message : String(error) };
  }
}
