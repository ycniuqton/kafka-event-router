import { ApiError, AuthMode, AuthProblemCode, FieldErrorMapper, ProblemKind, type AuthConfig } from '../../api/http/core';
import { LoginErrorText } from './AuthPaths';

/** What a failed password login shows: a banner message and/or messages per form field. */
export type LoginFailure = { message: string | null; fields: Record<string, string> };

/** Turns password-login problems into copy: 401 invalid credentials, 429 locked (Retry-After), 403 disabled, 422 fields. */
export class LoginProblem {
  static describe(error: unknown): LoginFailure {
    if (!(error instanceof ApiError)) return { message: LoginErrorText.fallback, fields: {} };
    const fields = error.is(ProblemKind.Validation) ? new FieldErrorMapper().map(error) : {};
    if (Object.keys(fields).length > 0) return { message: null, fields };
    return { message: LoginProblem.message(error), fields };
  }

  private static message(error: ApiError): string {
    if (error.code === AuthProblemCode.LoginLocked || error.is(ProblemKind.RateLimited)) return LoginErrorText.locked(error.retryAfter);
    if (error.code === AuthProblemCode.Disabled || error.is(ProblemKind.Forbidden)) return LoginErrorText.disabled;
    if (error.code === AuthProblemCode.InvalidCredentials || error.is(ProblemKind.Unauthenticated)) return LoginErrorText.invalidCredentials;
    return error.message;
  }
}

/** Sign-in methods offered by `/auth/config`, in screen order (password form first, then SSO, then dev login). */
export const LoginMethod = { Password: AuthMode.Password, Oidc: AuthMode.Oidc, Dev: AuthMode.Dev } as const;
export type LoginMethod = (typeof LoginMethod)[keyof typeof LoginMethod];

export class LoginMethods {
  static readonly order: readonly LoginMethod[] = [LoginMethod.Password, LoginMethod.Oidc, LoginMethod.Dev];

  static of(config: AuthConfig): LoginMethod[] {
    return LoginMethods.order.filter((m) => config.modes.includes(m));
  }
}
