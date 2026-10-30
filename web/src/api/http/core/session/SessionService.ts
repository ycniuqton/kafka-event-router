import type { ApiSchemas } from '../../../generated';

export type AuthConfig = ApiSchemas['AuthConfig'];
export type SessionUser = ApiSchemas['CurrentUser'];
export type TeamRole = ApiSchemas['TeamRole'];
export type PasswordCredentials = ApiSchemas['PasswordLogin'];
export type PasswordChangeBody = ApiSchemas['PasswordChange'];

/** Login modes advertised by `GET /auth/config`. */
export const AuthMode = { Oidc: 'oidc', Dev: 'dev', ApiToken: 'api_token', Password: 'password' } as const;
export type AuthMode = (typeof AuthMode)[keyof typeof AuthMode];

/** Browser-facing auth endpoints (served at the site root, not under `/api/v1`). */
export const AuthEndpoint = {
  Config: '/auth/config',
  OidcLogin: '/auth/oidc/login',
  DevLogin: '/auth/dev/login',
  PasswordLogin: '/auth/password/login',
  Logout: '/auth/logout',
  Me: '/auth/me',
  PasswordChange: '/auth/password',
} as const;

/** `code` members of the login / password problems (Go `password.ProblemCode`, OIDC `disabled`). */
export const AuthProblemCode = {
  InvalidCredentials: 'invalid_credentials',
  LoginLocked: 'login_locked',
  Disabled: 'disabled',
  PasswordChangeRequired: 'password_change_required',
  InvalidCurrentPassword: 'invalid_current_password',
  WeakPassword: 'weak_password',
  PasswordReused: 'password_reused',
} as const;
export type AuthProblemCode = (typeof AuthProblemCode)[keyof typeof AuthProblemCode];

/** Query key of the post-login destination. */
export const ReturnToParam = 'return_to';

/** The signed-in session: who am I, how can I sign in, sign in (dev) and sign out. */
export abstract class SessionService {
  /** Current principal; rejects with an `unauthenticated` ApiError when signed out. */
  abstract me(): Promise<SessionUser>;
  abstract config(): Promise<AuthConfig>;
  abstract devLogin(email?: string): Promise<void>;
  /** Email + password login at `loginUrl` (from `/auth/config`); sets the session cookies. */
  abstract passwordLogin(credentials: PasswordCredentials, loginUrl?: string): Promise<void>;
  /** Changes the caller's password; the server rotates the session, so this resolves with the fresh `/auth/me`. */
  abstract changePassword(body: PasswordChangeBody): Promise<SessionUser>;
  abstract logout(): Promise<void>;
  /** Absolute-path URL that starts SSO and comes back to `returnTo`. */
  abstract oidcLoginUrl(returnTo: string): string;
}
