import { describe, expect, it } from 'vitest';
import { ApiError, AuthMode, ProblemCatalog, ProblemKind, type AuthConfig } from '../../api/http/core';
import { PasswordRules } from '../../domain';
import { AuthPaths, LoginErrorText, RetryWait } from './AuthPaths';
import { PasswordMode } from './AuthConfigStore';
import { LoginMethod, LoginMethods, LoginProblem } from './LoginProblem';

const problem = (kind: ProblemKind, status: number, extra: Record<string, unknown> = {}) =>
  ApiError.fromProblem({ type: ProblemCatalog.typeOf(kind), title: kind, status, ...extra });

const config = (modes: AuthConfig['modes'], extra: Partial<AuthConfig> = {}): AuthConfig => ({ modes, workspace: 'w', ...extra });

describe('LoginMethods', () => {
  it('offers the password form first, then SSO, then dev login, only for enabled modes', () => {
    expect(LoginMethods.of(config([AuthMode.Oidc, AuthMode.Password, AuthMode.ApiToken]))).toEqual([LoginMethod.Password, LoginMethod.Oidc]);
    expect(LoginMethods.of(config([AuthMode.Dev, AuthMode.Oidc]))).toEqual([LoginMethod.Oidc, LoginMethod.Dev]);
    expect(LoginMethods.of(config([AuthMode.ApiToken]))).toEqual([]);
  });
});

describe('PasswordMode', () => {
  it('reads password mode and its policy (default min length when absent)', () => {
    const on = config([AuthMode.Password], { password_login_url: '/auth/password/login', password_policy: { min_length: 16 } });
    expect(PasswordMode.enabled(on)).toBe(true);
    expect(PasswordMode.policy(on).minLength).toBe(16);
    expect(PasswordMode.enabled(config([AuthMode.Oidc]))).toBe(false);
    expect(PasswordMode.policy(null)).toEqual(PasswordRules.policy());
  });

  it('keeps email invitations unless the server is password-only', () => {
    expect(PasswordMode.invitesApply(config([AuthMode.Oidc]))).toBe(true);
    expect(PasswordMode.invitesApply(config([AuthMode.Password, AuthMode.Oidc]))).toBe(true);
    expect(PasswordMode.invitesApply(config([AuthMode.Password, AuthMode.ApiToken]))).toBe(false);
  });
});

describe('LoginProblem', () => {
  it('never reveals whether the email exists on 401', () => {
    const failure = LoginProblem.describe(problem(ProblemKind.Unauthenticated, 401, { code: 'invalid_credentials', detail: 'x' }));
    expect(failure).toEqual({ message: LoginErrorText.invalidCredentials, fields: {} });
  });

  it('shows the lockout wait from retry_after', () => {
    const failure = LoginProblem.describe(problem(ProblemKind.RateLimited, 429, { code: 'login_locked', retry_after: 300 }));
    expect(failure.message).toBe('Too many sign-in attempts. Try again in 5 minutes.');
    expect(LoginProblem.describe(problem(ProblemKind.RateLimited, 429)).message).toBe('Too many sign-in attempts. Try again later.');
  });

  it('shows the disabled-account message on 403 (same as the OIDC ?error=disabled)', () => {
    expect(LoginProblem.describe(problem(ProblemKind.Forbidden, 403, { code: 'disabled' })).message).toBe(LoginErrorText.disabled);
    expect(LoginErrorText.of('disabled')).toBe(LoginErrorText.disabled);
  });

  it('puts 422 problems on the email / password fields', () => {
    const error = problem(ProblemKind.Validation, 422, { errors: [{ path: '/email', code: 'invalid_email', message: 'Enter a valid email.' }] });
    expect(LoginProblem.describe(error)).toEqual({ message: null, fields: { email: 'Enter a valid email.' } });
  });

  it('falls back to a generic message for non-API failures', () => {
    expect(LoginProblem.describe(new Error('boom')).message).toBe(LoginErrorText.fallback);
  });
});

describe('RetryWait', () => {
  it('formats seconds and minutes', () => {
    expect(RetryWait.text(1)).toBe('in 1 second');
    expect(RetryWait.text(45)).toBe('in 45 seconds');
    expect(RetryWait.text(61)).toBe('in 2 minutes');
    expect(RetryWait.text(undefined)).toBe(RetryWait.unknown);
    expect(AuthPaths.login).toBe('/login');
  });
});
