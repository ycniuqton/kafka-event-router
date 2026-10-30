import {
  ApiError, AuthEndpoint, AuthMode, AuthProblemCode, HttpStatus, ProblemCatalog, ProblemKind, SessionService,
  type AuthConfig, type PasswordChangeBody, type SessionUser,
} from '../../api/http/core';
import { PasswordChangeField, PasswordRules } from '../../domain';
import { AuthPaths } from './AuthPaths';

/** 422 problems of the mock password change (same codes and paths as the server). */
class MockPasswordProblem {
  static readonly weak = 'Use a password of at least 12 and at most 256 characters.';
  static readonly reused = 'Choose a password different from the current one.';

  static of(body: PasswordChangeBody): ApiError | null {
    if (PasswordRules.issueOf(body.new_password, PasswordRules.policy())) return MockPasswordProblem.problem(AuthProblemCode.WeakPassword, MockPasswordProblem.weak);
    if (body.new_password === body.current_password) return MockPasswordProblem.problem(AuthProblemCode.PasswordReused, MockPasswordProblem.reused);
    return null;
  }

  private static problem(code: string, detail: string): ApiError {
    const type = ProblemCatalog.typeOf(ProblemKind.Validation);
    return ApiError.fromProblem({ type, title: ProblemKind.Validation, status: HttpStatus.UnprocessableEntity, detail, code, path: `/${PasswordChangeField.Next}` });
  }
}

/** Mock data source: always signed in as the demo admin (password mode on), so the mock app never shows the login screen. */
export class MockSessionService extends SessionService {
  static readonly user: SessionUser = {
    id: '00000000-0000-0000-0000-000000000001',
    email: 'dev@eventrouter.local',
    display_name: 'Dev Admin',
    initials: 'DA',
    teams: [],
    workspace: 'Demo',
    csrf_token: '',
    must_change_password: false,
  };

  me(): Promise<SessionUser> {
    return Promise.resolve(MockSessionService.user);
  }

  config(): Promise<AuthConfig> {
    return Promise.resolve({
      modes: [AuthMode.Dev, AuthMode.Password],
      password_login_url: AuthEndpoint.PasswordLogin,
      password_policy: { min_length: PasswordRules.policy().minLength },
      workspace: MockSessionService.user.workspace,
    });
  }

  devLogin(): Promise<void> {
    return Promise.resolve();
  }

  passwordLogin(): Promise<void> {
    return Promise.resolve();
  }

  /** Any current password is accepted; weak and reused new passwords are rejected like the server does. */
  changePassword(body: PasswordChangeBody): Promise<SessionUser> {
    const problem = MockPasswordProblem.of(body);
    return problem ? Promise.reject(problem) : this.me();
  }

  logout(): Promise<void> {
    return Promise.resolve();
  }

  oidcLoginUrl(): string {
    return AuthPaths.home;
  }
}
