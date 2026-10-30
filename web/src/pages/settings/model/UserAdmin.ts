import { services } from '../../../api';
import { ApiError, FieldErrorMapper } from '../../../api/http/core';
import {
  PasswordRules, PasswordSource, RoleKey, Tone, UserStatus,
  type CreatedUser, type NewUserDraft, type PasswordPolicy, type PasswordResetDraft, type PasswordResetResult, type TeamMember,
} from '../../../domain';

/** Copy of Settings → Users. */
export const UserAdminText = {
  Title: 'Users',
  Hint: 'Disable someone to sign them out everywhere and block sign-in.',
  PasswordHint: 'Created users get a temporary password and must change it at first sign-in.',
  Create: '+ Create user',
  CreateTitle: 'Create user',
  CreateSubtitle: 'They must change this password at first sign-in. There is no self sign-up.',
  ResetTitle: 'Reset password',
  ResetSubtitle: 'They are signed out everywhere and must change the new password at next sign-in.',
  DisableTitle: 'Disable this user?',
  DisableBody: 'They are signed out everywhere and can no longer sign in. You can enable them again later.',
  Reset: 'Reset password',
  Disable: 'Disable',
  Enable: 'Enable',
  Cancel: 'Cancel',
  Done: 'Done',
  Email: 'Email',
  DisplayName: 'Display name',
  Password: 'Password',
  Generate: 'Generate',
  Manual: 'Set password',
  GenerateHint: 'A strong password is generated and shown once.',
  WorkspaceAdmin: 'Workspace admin (manage users, teams, settings, audit)',
  Team: 'Team',
  NoTeam: 'No team yet',
  Role: 'Role',
  GeneratedTitle: 'Copy the password now',
  GeneratedBody: "It won't be shown again. Send it to the user over a secure channel; they must change it at first sign-in.",
  Copy: 'Copy',
  Copied: 'Password copied',
  Required: 'Required.',
  SignInPassword: 'Password',
  SignInSso: 'SSO',
  Created: (email: string) => `User ${email} created`,
  ResetDone: (name: string) => `${name} must change the new password at next sign-in`,
  Disabled: (name: string) => `${name} is disabled and was signed out`,
  Enabled: (name: string) => `${name} can sign in again`,
} as const;

/** Badges of a user row. */
export const UserBadgeMeta = {
  WorkspaceAdmin: { label: 'Workspace admin', tone: Tone.Accent },
  Invited: { label: 'Invited', tone: Tone.Info },
  Disabled: { label: 'Disabled', tone: Tone.Off },
  MustChange: { label: 'Must change password', tone: Tone.Warn },
} as const;
export type UserBadge = (typeof UserBadgeMeta)[keyof typeof UserBadgeMeta];

/** Fields of the create-user form (the API names, so problem paths map onto them). */
export const NewUserField = { Email: 'email', DisplayName: 'displayName', Password: 'password', TeamId: 'teamId', Role: 'role' } as const;
export type NewUserField = (typeof NewUserField)[keyof typeof NewUserField];
export type FieldMessages = Partial<Record<string, string>>;

/** What a failed admin call shows: messages per field and/or a banner. */
export type AdminFailure = { fields: FieldMessages; message: string | null };

/** Settings → Users: one row per user, badges, create / reset password / disable–enable, and their problem copy. */
export class UserAdmin {
  /** The first member row of each user (member rows repeat per team binding). */
  static users(rows: readonly TeamMember[]): TeamMember[] {
    return rows.filter((r, index) => rows.findIndex((o) => o.userId === r.userId) === index);
  }

  static badges(user: TeamMember): UserBadge[] {
    const out: UserBadge[] = [];
    if (user.workspaceAdmin) out.push(UserBadgeMeta.WorkspaceAdmin);
    if (user.status === UserStatus.Invited) out.push(UserBadgeMeta.Invited);
    if (user.status === UserStatus.Disabled) out.push(UserBadgeMeta.Disabled);
    if (user.mustChangePassword) out.push(UserBadgeMeta.MustChange);
    return out;
  }

  static signIn(user: TeamMember): string {
    return user.passwordLogin ? UserAdminText.SignInPassword : UserAdminText.SignInSso;
  }

  static isDisabled(user: TeamMember): boolean {
    return user.status === UserStatus.Disabled;
  }

  static blankDraft(): NewUserDraft {
    return { email: '', displayName: '', passwordSource: PasswordSource.Generate, password: '', workspaceAdmin: false, teamId: null, role: RoleKey.Viewer };
  }

  static blankReset(): PasswordResetDraft {
    return { passwordSource: PasswordSource.Generate, password: '' };
  }

  /** Local checks before `POST /users` (the server re-checks everything). */
  static draftErrors(draft: NewUserDraft, policy: PasswordPolicy): FieldMessages {
    const out: FieldMessages = {};
    if (!draft.email.trim()) out[NewUserField.Email] = UserAdminText.Required;
    if (!draft.displayName.trim()) out[NewUserField.DisplayName] = UserAdminText.Required;
    const issue = PasswordRules.chosenIssue(draft.passwordSource, draft.password, policy);
    if (issue) out[NewUserField.Password] = PasswordRules.text(issue, policy);
    return out;
  }

  static resetErrors(draft: PasswordResetDraft, policy: PasswordPolicy): FieldMessages {
    const issue = PasswordRules.chosenIssue(draft.passwordSource, draft.password, policy);
    return issue ? { [NewUserField.Password]: PasswordRules.text(issue, policy) } : {};
  }

  static create(draft: NewUserDraft): Promise<CreatedUser> {
    return services.access.createUser(draft);
  }

  static reset(user: TeamMember, draft: PasswordResetDraft): Promise<PasswordResetResult> {
    return services.access.resetPassword(user, draft);
  }

  /** Disables (sessions revoked) or re-enables a user; resolves with the toast text. */
  static async setDisabled(user: TeamMember, disabled: boolean): Promise<string> {
    await services.access.updateUser(user, { disabled });
    return disabled ? UserAdminText.Disabled(user.name) : UserAdminText.Enabled(user.name);
  }

  /** Server problems: JSON Pointer paths land on their fields, the rest in the banner. */
  static failure(error: unknown): AdminFailure {
    if (!(error instanceof ApiError)) return { fields: {}, message: error instanceof Error ? error.message : String(error) };
    const fields = new FieldErrorMapper().map(error);
    return { fields, message: Object.keys(fields).length > 0 ? null : error.message };
  }
}
