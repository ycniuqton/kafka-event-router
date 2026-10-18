import type { BrowserContext, Page } from '@playwright/test';
import { LoginMethod, Profile, type TargetProfile } from './Target';

/** Auth endpoints and UI texts the login strategies use. */
export const AuthPaths = {
  devLogin: '/auth/dev/login',
  oidcLogin: '/auth/oidc/login',
  loginPage: '/login',
  me: '/api/v1/auth/me',
  logout: '/auth/logout',
} as const;

/** Timeouts of the sign-in flows. */
const LoginTiming = { pageMs: 45_000 } as const;

/** One way of signing a browser context in. */
export abstract class LoginStrategy {
  constructor(protected readonly profile: TargetProfile) {}

  /** Signs the context in; email selects a user where the method supports it. */
  abstract signIn(context: BrowserContext, email?: string): Promise<void>;

  /** Signs in through the visible login page (UI tests). */
  abstract signInPage(page: Page): Promise<void>;
}

/** Local dev login: POST /auth/dev/login seeds the user (admin of the default team on first use). */
export class DevLogin extends LoginStrategy {
  async signIn(context: BrowserContext, email?: string): Promise<void> {
    const res = await context.request.post(new URL(AuthPaths.devLogin, this.profile.baseURL).toString(), { data: email ? { email } : {} });
    if (!res.ok()) throw new Error(`dev login failed: ${res.status()} ${await res.text()}`);
  }

  async signInPage(page: Page): Promise<void> {
    await page.goto(AuthPaths.loginPage);
    const button = page.getByRole('button', { name: 'Dev login' });
    await button.waitFor({ timeout: LoginTiming.pageMs });
    await button.click();
    await page.waitForURL((url) => !url.pathname.startsWith(AuthPaths.loginPage));
  }
}

/** Staging: OIDC through Dex with the static user from the credentials file (one user only). */
export class DexOidcLogin extends LoginStrategy {
  async signIn(context: BrowserContext, email?: string): Promise<void> {
    if (email) throw new Error('Dex OIDC login has a single static user; impersonation is not available');
    const page = await context.newPage();
    try {
      await page.goto(AuthPaths.oidcLogin);
      await this.fillDexForm(page);
    } finally {
      await page.close();
    }
  }

  async signInPage(page: Page): Promise<void> {
    await page.goto(AuthPaths.oidcLogin);
    await this.fillDexForm(page);
  }

  private async fillDexForm(page: Page): Promise<void> {
    const login = page.locator('input[name="login"]');
    await login.waitFor({ timeout: LoginTiming.pageMs });
    await login.fill(this.profile.credentials.email);
    await page.locator('input[name="password"]').fill(this.profile.credentials.password);
    await page.locator('button[type="submit"]').click();
    await page.waitForURL((url) => !url.pathname.startsWith('/dex') && !url.pathname.startsWith(AuthPaths.loginPage), { timeout: LoginTiming.pageMs });
  }
}

/** Selects the strategy configured for the target (no branching on concrete types in tests). */
export class LoginManager {
  private static readonly strategies: Record<LoginMethod, new (p: TargetProfile) => LoginStrategy> = {
    [LoginMethod.Dev]: DevLogin,
    [LoginMethod.DexOidc]: DexOidcLogin,
  };

  static forProfile(profile: TargetProfile = Profile): LoginStrategy {
    return new LoginManager.strategies[profile.login](profile);
  }
}
