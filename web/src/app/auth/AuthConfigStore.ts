import { AuthMode, type AuthConfig } from '../../api/http/core';
import { PasswordRules, type PasswordPolicy } from '../../domain';
import { SessionServices } from './SessionServices';

/** Loads the public `/auth/config` once per page load (a failed load is retried on the next use). */
export class AuthConfigStore {
  private static pending: Promise<AuthConfig> | null = null;

  static load(): Promise<AuthConfig> {
    AuthConfigStore.pending ??= SessionServices.current()
      .config()
      .catch((error: unknown) => {
        AuthConfigStore.pending = null;
        throw error;
      });
    return AuthConfigStore.pending;
  }

  static reset(): void {
    AuthConfigStore.pending = null;
  }
}

/** Password-mode facts of an auth config: is it on, which policy, where to post the login. */
export class PasswordMode {
  static enabled(config: AuthConfig | null | undefined): boolean {
    return config?.modes.includes(AuthMode.Password) ?? false;
  }

  /** Email invitations are claimed at first SSO/dev sign-in; a password-only server creates users instead. */
  static invitesApply(config: AuthConfig | null | undefined): boolean {
    return !PasswordMode.enabled(config) || (config?.modes.some((m) => m !== AuthMode.Password && m !== AuthMode.ApiToken) ?? true);
  }

  static policy(config: AuthConfig | null | undefined): PasswordPolicy {
    return PasswordRules.policy(config?.password_policy?.min_length);
  }
}
