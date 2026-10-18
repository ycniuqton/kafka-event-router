import { ApiError } from '../../../api/http/core';
import { Format } from '../../../components/ui/Format';
import { ApiTokenStatus, ApiTokenRules, Tone, type ApiToken, type ApiTokenDraft, type ApiTokenScope } from '../../../domain';

/** Copy of the API tokens panel. */
export const ApiTokenText = {
  title: 'API tokens',
  hint: 'Tokens act as you, limited by their scope. Send them as Authorization: Bearer <token>.',
  create: 'New token',
  creating: 'Creating…',
  empty: 'No API tokens yet. Create one for scripts and CI.',
  loading: 'Loading tokens…',
  unavailable: 'Tokens could not be loaded. Reload to retry.',
  revoke: 'Revoke',
  revokeTitle: 'Revoke this token?',
  revokeBody: 'Scripts using it stop working immediately. This cannot be undone.',
  revoked: 'Token revoked',
  cancel: 'Cancel',
  modalTitle: 'New API token',
  modalSubtitle: 'The token is shown once. Store it in your secret manager right away.',
  name: 'Name',
  namePlaceholder: 'ci-deploy',
  scope: 'Scope',
  expires: 'Expires after',
  secretTitle: 'Copy your token now',
  secretBody: "You won't be able to see it again. Revoke it and create a new one if it's lost.",
  copy: 'Copy',
  copied: 'Token copied',
  done: 'Done',
  neverUsed: 'Never used',
  usedPrefix: 'Used ',
  expired: 'Expired',
  expiresPrefix: 'Expires ',
  noExpiry: 'No expiry',
  days: ' days',
} as const;

/** One listed token, ready to render. */
export type ApiTokenRow = {
  id: string;
  name: string;
  prefix: string;
  scopes: ApiTokenScope[];
  used: string;
  expiry: string;
  expiryTone: Tone;
};

/** Builds token rows and form feedback for the panel. */
export class ApiTokenRows {
  static readonly columns = '1fr 130px 90px 110px 120px 80px';
  static readonly messageColumns = '1fr';

  static rows(tokens: ApiToken[], now: Date): ApiTokenRow[] {
    return tokens.map((t) => ({
      id: t.id,
      name: t.name,
      prefix: `${t.prefix}…`,
      scopes: t.scopes,
      used: t.lastUsedAt ? ApiTokenText.usedPrefix + Format.relativeTime(t.lastUsedAt, now) : ApiTokenText.neverUsed,
      expiry: ApiTokenRows.expiry(t),
      expiryTone: t.status === ApiTokenStatus.Expired ? Tone.Err : Tone.Off,
    }));
  }

  static expiry(t: ApiToken): string {
    if (t.status === ApiTokenStatus.Expired) return ApiTokenText.expired;
    if (!t.expiresAt) return ApiTokenText.noExpiry;
    return ApiTokenText.expiresPrefix + Format.shortDate(t.expiresAt);
  }

  static expiryOptions(): { value: string; label: string }[] {
    return ApiTokenRules.expiryDays.map((d) => ({ value: String(d), label: d + ApiTokenText.days }));
  }

  /** The message to show under the name field for a failed create (server field errors or generic detail). */
  static createError(error: unknown, draft: ApiTokenDraft): string {
    const local = ApiTokenRules.nameProblem(draft);
    if (local) return local;
    if (error instanceof ApiError) return error.fieldErrors()['/name'] ?? error.message;
    return error instanceof Error ? error.message : String(error);
  }
}
