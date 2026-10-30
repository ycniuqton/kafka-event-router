import { describe, expect, it } from 'vitest';
import { ApiError, ProblemCatalog, ProblemKind } from '../../../api/http/core';
import { PasswordRules, PasswordSource, UserStatus, type TeamMember } from '../../../domain';
import { MEMBER_DIRECTORY } from '../../../mocks';
import { NewUserField, UserAdmin, UserAdminText, UserBadgeMeta } from './UserAdmin';

const base = MEMBER_DIRECTORY[0];
const row = (extra: Partial<TeamMember>): TeamMember => ({ ...base, ...extra });
const policy = PasswordRules.policy(12);

describe('UserAdmin', () => {
  it('lists each user once (member rows repeat per team)', () => {
    const rows = [row({ userId: 'a', teamId: 't1' }), row({ userId: 'a', teamId: 't2' }), row({ userId: 'b' })];
    expect(UserAdmin.users(rows).map((r) => [r.userId, r.teamId])).toEqual([['a', 't1'], ['b', base.teamId]]);
  });

  it('badges admins, disabled users and pending password changes', () => {
    expect(UserAdmin.badges(row({ workspaceAdmin: true, status: UserStatus.Disabled, mustChangePassword: true }))).toEqual([
      UserBadgeMeta.WorkspaceAdmin, UserBadgeMeta.Disabled, UserBadgeMeta.MustChange,
    ]);
    expect(UserAdmin.badges(row({ workspaceAdmin: false, mustChangePassword: false }))).toEqual([]);
    expect(UserAdmin.signIn(row({ passwordLogin: false }))).toBe(UserAdminText.SignInSso);
  });

  it('checks the create form locally (generate needs no password)', () => {
    const draft = { ...UserAdmin.blankDraft(), email: ' ', displayName: '' };
    expect(UserAdmin.draftErrors(draft, policy)).toEqual({ [NewUserField.Email]: UserAdminText.Required, [NewUserField.DisplayName]: UserAdminText.Required });
    const typed = { ...draft, email: 'a@x.io', displayName: 'A', passwordSource: PasswordSource.Manual, password: 'short' };
    expect(UserAdmin.draftErrors(typed, policy)).toEqual({ [NewUserField.Password]: 'Use at least 12 characters.' });
    expect(UserAdmin.resetErrors(UserAdmin.blankReset(), policy)).toEqual({});
  });

  it('maps server problems to fields (409/422) or a banner', () => {
    const exists = ApiError.fromProblem({ type: ProblemCatalog.typeOf(ProblemKind.Conflict), title: 'c', status: 409, code: 'user_exists', detail: 'Already a member.', path: '/email' });
    expect(UserAdmin.failure(exists)).toEqual({ fields: { email: 'Already a member.' }, message: null });
    expect(UserAdmin.failure(new Error('offline'))).toEqual({ fields: {}, message: 'offline' });
  });
});
