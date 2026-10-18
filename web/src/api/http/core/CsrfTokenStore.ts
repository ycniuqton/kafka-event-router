/** Holds the double-submit CSRF token: from `/auth/me`, else from the readable `er_csrf` cookie. */
export class CsrfTokenStore {
  static readonly cookieName = 'er_csrf';
  private static readonly cookieSeparator = ';';
  private static readonly pairSeparator = '=';
  private token: string | null = null;

  set(token: string | null): void {
    this.token = token;
  }

  current(): string | null {
    return this.token ?? CsrfTokenStore.fromCookie();
  }

  private static fromCookie(): string | null {
    if (typeof document === 'undefined') return null;
    const prefix = CsrfTokenStore.cookieName + CsrfTokenStore.pairSeparator;
    const pair = document.cookie
      .split(CsrfTokenStore.cookieSeparator)
      .map((c) => c.trim())
      .find((c) => c.startsWith(prefix));
    return pair ? decodeURIComponent(pair.slice(prefix.length)) : null;
  }
}
