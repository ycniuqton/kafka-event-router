import { describe, expect, it, vi } from 'vitest';
import { ApiError, ProblemCatalog, ProblemKind, type PasswordChangeBody } from '../../api/http/core';
import { PasswordRules } from '../../domain';
import { MockSessionService } from './MockSessionService';
import { PasswordChange } from './PasswordChange';

const policy = PasswordRules.policy(12);
const draft = (current: string, next: string, confirm = next) => ({ current_password: current, new_password: next, confirm_password: confirm });

describe('PasswordChange', () => {
  it('checks length, confirmation and reuse before calling the server', async () => {
    const session = new MockSessionService();
    const spy = vi.spyOn(session, 'changePassword');
    const change = new PasswordChange(session, policy);
    const outcome = await change.submit(draft('', 'short', 'other'));
    expect(outcome.errors).toEqual({
      current_password: 'Enter a password.',
      new_password: 'Use at least 12 characters.',
      confirm_password: "The passwords don't match.",
    });
    expect(change.validate(draft('same-password-1', 'same-password-1')).new_password).toBe('Choose a password different from the current one.');
    expect(spy).not.toHaveBeenCalled();
  });

  it('sends current/new and resolves with the refreshed session user', async () => {
    const session = new MockSessionService();
    const spy = vi.spyOn(session, 'changePassword');
    const outcome = await new PasswordChange(session, policy).submit(draft('old-password', 'a-much-longer-password'));
    expect(spy).toHaveBeenCalledWith({ current_password: 'old-password', new_password: 'a-much-longer-password' } satisfies PasswordChangeBody);
    expect(outcome).toEqual({ user: MockSessionService.user, errors: {}, message: null });
  });

  it('maps server problems onto their fields (invalid_current_password)', async () => {
    const session = new MockSessionService();
    const error = ApiError.fromProblem({
      type: ProblemCatalog.typeOf(ProblemKind.Validation), title: 'v', status: 422, code: 'invalid_current_password',
      detail: 'The current password is incorrect.', path: '/current_password',
    });
    vi.spyOn(session, 'changePassword').mockRejectedValue(error);
    const outcome = await new PasswordChange(session, policy).submit(draft('wrong-password', 'a-much-longer-password'));
    expect(outcome).toEqual({ user: null, errors: { current_password: 'The current password is incorrect.' }, message: null });
  });

  it('mock session rejects weak and reused passwords like the server', async () => {
    const session = new MockSessionService();
    const weak = await new PasswordChange(session, PasswordRules.policy(1)).submit(draft('x', 'short'));
    expect(weak.errors.new_password).toBe('Use a password of at least 12 and at most 256 characters.');
  });
});
