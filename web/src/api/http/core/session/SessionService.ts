import type { ApiSchemas } from '../../../generated';

export type AuthConfig = ApiSchemas['AuthConfig'];
export type SessionUser = ApiSchemas['CurrentUser'];
export type TeamRole = ApiSchemas['TeamRole'];

/** Login modes advertised by `GET /auth/config`. */
export const AuthMode = { Oidc: 'oidc', Dev: 'dev', ApiToken: 'api_token' } as const;
export type AuthMode = (typeof AuthMode)[keyof typeof AuthMode];

/** Browser-facing auth endpoints (served at the site root, not under `/api/v1`). */
export const AuthEndpoint = {
  Config: '/auth/config',
  OidcLogin: '/auth/oidc/login',
  DevLogin: '/auth/dev/login',
  Logout: '/auth/logout',
  Me: '/auth/me',
} as const;

/** Query key of the post-login destination. */
export const ReturnToParam = 'return_to';

/** The signed-in session: who am I, how can I sign in, sign in (dev) and sign out. */
export abstract class SessionService {
  /** Current principal; rejects with an `unauthenticated` ApiError when signed out. */
  abstract me(): Promise<SessionUser>;
  abstract config(): Promise<AuthConfig>;
  abstract devLogin(email?: string): Promise<void>;
  abstract logout(): Promise<void>;
  /** Absolute-path URL that starts SSO and comes back to `returnTo`. */
  abstract oidcLoginUrl(returnTo: string): string;
}
