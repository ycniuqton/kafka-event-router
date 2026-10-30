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

/** Messages for the `error` codes the OIDC callback redirects with, and for password login problems. */
export class LoginErrorText {
  static readonly fallback = 'Sign-in failed. Try again.';
  static readonly invalidCredentials = 'The email or password is incorrect.';
  static readonly disabled = 'This account is disabled. Ask a workspace admin to enable it.';
  private static readonly byCode: Readonly<Record<string, string>> = {
    state: 'Your sign-in session expired. Start again.',
    idp_unavailable: "Can't reach the identity provider. Try again in a moment.",
    invalid_token: 'The identity provider response could not be verified.',
    conflict: 'This email is already linked to another account. Ask an admin to unlink it.',
    disabled: LoginErrorText.disabled,
  };

  /** Too many failed attempts: the account (or this client) is locked for `seconds`. */
  static locked(seconds: number | undefined): string {
    return `Too many sign-in attempts. Try again ${RetryWait.text(seconds)}.`;
  }

  static of(code: string | null): string | null {
    if (!code) return null;
    return LoginErrorText.byCode[code] ?? LoginErrorText.fallback;
  }
}

/** "in 30 seconds" / "in 5 minutes" / "later" for a `Retry-After` value. */
export class RetryWait {
  static readonly secondsPerMinute = 60;
  static readonly unknown = 'later';

  static text(seconds: number | undefined): string {
    if (!seconds || seconds <= 0) return RetryWait.unknown;
    if (seconds < RetryWait.secondsPerMinute) return `in ${RetryWait.plural(Math.ceil(seconds), 'second')}`;
    return `in ${RetryWait.plural(Math.ceil(seconds / RetryWait.secondsPerMinute), 'minute')}`;
  }

  private static plural(n: number, unit: string): string {
    return `${n} ${unit}${n === 1 ? '' : 's'}`;
  }
}

/** The browser's current in-app path (path + query), the default `return_to`. */
export class CurrentLocation {
  static path(): string {
    return window.location.pathname + window.location.search;
  }
}
