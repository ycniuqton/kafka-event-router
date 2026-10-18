import { UrlBuilder } from '../HttpMessages';
import type { HttpContext } from '../HttpContext';
import { AuthEndpoint, ReturnToParam, SessionService, type AuthConfig, type SessionUser } from './SessionService';

/** Session over the real auth endpoints; keeps the CSRF token from `/auth/me` for later writes. */
export class HttpSessionService extends SessionService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async me(): Promise<SessionUser> {
    const user = await this.context.api.get<SessionUser>(AuthEndpoint.Me, undefined, { silent: true });
    this.context.csrf.set(user.csrf_token);
    return user;
  }

  config(): Promise<AuthConfig> {
    return this.context.auth.get<AuthConfig>(AuthEndpoint.Config, undefined, { silent: true });
  }

  async devLogin(email?: string): Promise<void> {
    await this.context.auth.post<void>(AuthEndpoint.DevLogin, email ? { email } : {}, { silent: true });
  }

  async logout(): Promise<void> {
    await this.context.auth.post<void>(AuthEndpoint.Logout);
    this.context.csrf.set(null);
  }

  oidcLoginUrl(returnTo: string): string {
    return UrlBuilder.build(this.context.auth.baseUrl, AuthEndpoint.OidcLogin, { [ReturnToParam]: returnTo });
  }
}
