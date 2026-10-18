/** What an API token may do on top of its owner's roles: read ⊂ write ⊂ admin. */
export const ApiTokenScope = { Read: 'read', Write: 'write', Admin: 'admin' } as const;
export type ApiTokenScope = (typeof ApiTokenScope)[keyof typeof ApiTokenScope];

/** Whether a listed token still authenticates. */
export const ApiTokenStatus = { Active: 'active', Expired: 'expired' } as const;
export type ApiTokenStatus = (typeof ApiTokenStatus)[keyof typeof ApiTokenStatus];

/** A personal API token as listed (the secret is never listed). */
export type ApiToken = {
  id: string;
  name: string;
  prefix: string;
  scopes: ApiTokenScope[];
  createdAt: string;
  expiresAt: string | null;
  lastUsedAt: string | null;
  status: ApiTokenStatus;
};

/** The create form. */
export type ApiTokenDraft = { name: string; scope: ApiTokenScope; expiresInDays: number };

/** A freshly created token: `secret` is shown once and never retrievable again. */
export type CreatedApiToken = { token: ApiToken; secret: string };

/** Business rules and copy of API tokens (mirrors the server policy). */
export class ApiTokenRules {
  static readonly maxNameLength = 100;
  static readonly defaultExpiryDays = 90;
  static readonly expiryDays = [7, 30, 90, 365] as const;
  static readonly scopes: ApiTokenScope[] = [ApiTokenScope.Read, ApiTokenScope.Write, ApiTokenScope.Admin];
  static readonly scopeHelp: Record<ApiTokenScope, string> = {
    read: 'Read only: GET requests, nothing changes.',
    write: 'Read and change routes, connections and other resources you can edit.',
    admin: 'Everything you can do, including admin actions.',
  };

  static emptyDraft(): ApiTokenDraft {
    return { name: '', scope: ApiTokenScope.Read, expiresInDays: ApiTokenRules.defaultExpiryDays };
  }

  /** Returns the problem with the draft's name, or null when it can be sent. */
  static nameProblem(draft: ApiTokenDraft): string | null {
    const name = draft.name.trim();
    if (name.length === 0) return 'Give the token a name, e.g. ci-deploy.';
    if (name.length > ApiTokenRules.maxNameLength) return `Use a shorter name (at most ${ApiTokenRules.maxNameLength} characters).`;
    return null;
  }
}
