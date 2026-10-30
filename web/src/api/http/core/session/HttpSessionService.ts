import { UrlBuilder } from '../HttpMessages';
import type { HttpContext } from '../HttpContext';
import { AuthEndpoint, ReturnToParam, SessionService, type AuthConfig, type PasswordChangeBody, type PasswordCredentials, type SessionUser } from './SessionService';

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

  async passwordLogin(credentials: PasswordCredentials, loginUrl: string = AuthEndpoint.PasswordLogin): Promise<void> {
    await this.context.auth.post<void>(loginUrl, credentials, { silent: true });
  }

  async changePassword(body: PasswordChangeBody): Promise<SessionUser> {
    await this.context.api.post<void>(AuthEndpoint.PasswordChange, body, { silent: true });
    return this.me();
  }

  async logout(): Promise<void> {
    await this.context.auth.post<void>(AuthEndpoint.Logout);
    this.context.csrf.set(null);
  }

  oidcLoginUrl(returnTo: string): string {
    return UrlBuilder.build(this.context.auth.baseUrl, AuthEndpoint.OidcLogin, { [ReturnToParam]: returnTo });
  }
}
