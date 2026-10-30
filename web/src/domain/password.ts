import type { RoleKey, TeamMember } from './member';

/** Password policy limits (server: `password_policy.min_length` from `/auth/config`, at most 256 characters). */
export const PasswordLimits = { DefaultMinLength: 12, MaxLength: 256 } as const;

export type PasswordPolicy = { minLength: number; maxLength: number };

/** Why a password is rejected before it is sent. */
export const PasswordIssue = { Missing: 'missing', TooShort: 'too_short', TooLong: 'too_long', Mismatch: 'mismatch', Reused: 'reused' } as const;
export type PasswordIssue = (typeof PasswordIssue)[keyof typeof PasswordIssue];

/** Fields of the change-password form (the API names, so server problem paths map onto them). */
export const PasswordChangeField = { Current: 'current_password', Next: 'new_password', Confirm: 'confirm_password' } as const;
export type PasswordChangeField = (typeof PasswordChangeField)[keyof typeof PasswordChangeField];

export type PasswordChangeDraft = Record<PasswordChangeField, string>;

/** Where a new or reset password comes from: the server generates it, or the admin types it. */
export const PasswordSource = { Generate: 'generate', Manual: 'manual' } as const;
export type PasswordSource = (typeof PasswordSource)[keyof typeof PasswordSource];

/** `POST /users` input (password mode): optional first team role. */
export type NewUserDraft = {
  email: string;
  displayName: string;
  passwordSource: PasswordSource;
  password: string;
  workspaceAdmin: boolean;
  teamId: string | null;
  role: RoleKey;
};

/** The created user's member rows, plus the server-generated password (shown once). */
export type CreatedUser = { members: TeamMember[]; generatedPassword: string | null };

/** `POST /users/{id}/password-reset` input. */
export type PasswordResetDraft = { passwordSource: PasswordSource; password: string };

export type PasswordResetResult = { generatedPassword: string | null };

/** Client-side mirror of the password policy: length bounds, confirmation and reuse. */
export class PasswordRules {
  static policy(minLength?: number): PasswordPolicy {
    return { minLength: minLength ?? PasswordLimits.DefaultMinLength, maxLength: PasswordLimits.MaxLength };
  }

  static issueOf(password: string, policy: PasswordPolicy): PasswordIssue | null {
    if (password.length === 0) return PasswordIssue.Missing;
    if (password.length < policy.minLength) return PasswordIssue.TooShort;
    if (password.length > policy.maxLength) return PasswordIssue.TooLong;
    return null;
  }

  static blankChange(): PasswordChangeDraft {
    return { current_password: '', new_password: '', confirm_password: '' };
  }

  /** Issues per field of a change-password draft (empty when it may be sent). */
  static changeIssues(draft: PasswordChangeDraft, policy: PasswordPolicy): Partial<Record<PasswordChangeField, PasswordIssue>> {
    const out: Partial<Record<PasswordChangeField, PasswordIssue>> = {};
    if (!draft.current_password) out.current_password = PasswordIssue.Missing;
    const next = PasswordRules.issueOf(draft.new_password, policy);
    if (next) out.new_password = next;
    else if (draft.new_password === draft.current_password) out.new_password = PasswordIssue.Reused;
    if (draft.confirm_password !== draft.new_password) out.confirm_password = PasswordIssue.Mismatch;
    return out;
  }

  /** The password to send, or null to let the server generate one. */
  static chosen(source: PasswordSource, password: string): string | null {
    return source === PasswordSource.Manual ? password : null;
  }

  /** Issue of an admin-chosen password (none when the server generates it). */
  static chosenIssue(source: PasswordSource, password: string, policy: PasswordPolicy): PasswordIssue | null {
    return source === PasswordSource.Manual ? PasswordRules.issueOf(password, policy) : null;
  }

  static text(issue: PasswordIssue, policy: PasswordPolicy): string {
    const texts: Record<PasswordIssue, string> = {
      missing: 'Enter a password.',
      too_short: `Use at least ${policy.minLength} characters.`,
      too_long: `Use at most ${policy.maxLength} characters.`,
      mismatch: "The passwords don't match.",
      reused: 'Choose a password different from the current one.',
    };
    return texts[issue];
  }
}
