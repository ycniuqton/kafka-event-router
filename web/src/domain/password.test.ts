import { describe, expect, it } from 'vitest';
import { PasswordIssue, PasswordLimits, PasswordRules, PasswordSource } from './password';

describe('PasswordRules', () => {
  const policy = PasswordRules.policy(12);

  it('defaults to the server policy (12..256)', () => {
    expect(PasswordRules.policy()).toEqual({ minLength: PasswordLimits.DefaultMinLength, maxLength: PasswordLimits.MaxLength });
  });

  it('checks the length bounds', () => {
    expect(PasswordRules.issueOf('', policy)).toBe(PasswordIssue.Missing);
    expect(PasswordRules.issueOf('a'.repeat(11), policy)).toBe(PasswordIssue.TooShort);
    expect(PasswordRules.issueOf('a'.repeat(12), policy)).toBeNull();
    expect(PasswordRules.issueOf('a'.repeat(257), policy)).toBe(PasswordIssue.TooLong);
  });

  it('only checks admin-typed passwords; generate sends none', () => {
    expect(PasswordRules.chosen(PasswordSource.Generate, 'ignored')).toBeNull();
    expect(PasswordRules.chosen(PasswordSource.Manual, 'typed-password')).toBe('typed-password');
    expect(PasswordRules.chosenIssue(PasswordSource.Generate, '', policy)).toBeNull();
    expect(PasswordRules.chosenIssue(PasswordSource.Manual, 'short', policy)).toBe(PasswordIssue.TooShort);
  });

  it('reports change-form issues per field', () => {
    const ok = { current_password: 'old', new_password: 'new-password-123', confirm_password: 'new-password-123' };
    expect(PasswordRules.changeIssues(ok, policy)).toEqual({});
    expect(PasswordRules.changeIssues({ ...ok, confirm_password: 'x' }, policy)).toEqual({ confirm_password: PasswordIssue.Mismatch });
    expect(PasswordRules.text(PasswordIssue.TooShort, policy)).toBe('Use at least 12 characters.');
  });
});
