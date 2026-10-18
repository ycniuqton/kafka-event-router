import { ReturnToParam } from '../../api/http/core';

/** Query keys of the login screen (`error` is set by the OIDC callback on failure). */
export const LoginQuery = { ReturnTo: ReturnToParam, Error: 'error' } as const;

/** Login URL building and `return_to` sanitising (relative in-app paths only). */
export class AuthPaths {
  static readonly login = '/login';
  static readonly home = '/';
  private static readonly protocolRelative = '//';

  static loginUrl(returnTo: string): string {
    const params = new URLSearchParams({ [LoginQuery.ReturnTo]: AuthPaths.safeReturnTo(returnTo) });
    return `${AuthPaths.login}?${params.toString()}`;
  }

  /** Keeps `value` only if it is a same-app relative path that is not the login page itself. */
  static safeReturnTo(value: string | null | undefined): string {
    if (!value || !value.startsWith(AuthPaths.home) || value.startsWith(AuthPaths.protocolRelative)) return AuthPaths.home;
    return value.startsWith(AuthPaths.login) ? AuthPaths.home : value;
  }
}

/** Messages for the `error` codes the OIDC callback redirects with. */
export class LoginErrorText {
  static readonly fallback = 'Sign-in failed. Try again.';
  private static readonly byCode: Readonly<Record<string, string>> = {
    state: 'Your sign-in session expired. Start again.',
    idp_unavailable: "Can't reach the identity provider. Try again in a moment.",
    invalid_token: 'The identity provider response could not be verified.',
    conflict: 'This email is already linked to another account. Ask an admin to unlink it.',
  };

  static of(code: string | null): string | null {
    if (!code) return null;
    return LoginErrorText.byCode[code] ?? LoginErrorText.fallback;
  }
}

/** The browser's current in-app path (path + query), the default `return_to`. */
export class CurrentLocation {
  static path(): string {
    return window.location.pathname + window.location.search;
  }
}
