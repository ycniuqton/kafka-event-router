import { ApiError, FieldErrorMapper, type SessionService, type SessionUser } from '../../api/http/core';
import { PasswordRules, type PasswordChangeDraft, type PasswordChangeField, type PasswordPolicy } from '../../domain';

/** Result of a change attempt: the refreshed session user on success, else messages per field and/or a banner. */
export type PasswordChangeOutcome = { user: SessionUser | null; errors: Partial<Record<PasswordChangeField, string>>; message: string | null };

/** Own password change: client-side policy checks, then `POST /auth/password` and a fresh `/auth/me` (new CSRF token). */
export class PasswordChange {
  private readonly session: SessionService;
  readonly policy: PasswordPolicy;

  constructor(session: SessionService, policy: PasswordPolicy) {
    this.session = session;
    this.policy = policy;
  }

  validate(draft: PasswordChangeDraft): Partial<Record<PasswordChangeField, string>> {
    const issues = PasswordRules.changeIssues(draft, this.policy);
    const out: Partial<Record<PasswordChangeField, string>> = {};
    for (const [field, issue] of Object.entries(issues)) out[field as PasswordChangeField] = PasswordRules.text(issue, this.policy);
    return out;
  }

  async submit(draft: PasswordChangeDraft): Promise<PasswordChangeOutcome> {
    const errors = this.validate(draft);
    if (Object.keys(errors).length > 0) return { user: null, errors, message: null };
    try {
      const user = await this.session.changePassword({ current_password: draft.current_password, new_password: draft.new_password });
      return { user, errors: {}, message: null };
    } catch (error) {
      return PasswordChange.failure(error);
    }
  }

  /** Server problems: `/current_password` and `/new_password` land on their fields, anything else in the banner. */
  static failure(error: unknown): PasswordChangeOutcome {
    if (!(error instanceof ApiError)) return { user: null, errors: {}, message: error instanceof Error ? error.message : String(error) };
    const errors = new FieldErrorMapper().map(error) as Partial<Record<PasswordChangeField, string>>;
    return { user: null, errors, message: Object.keys(errors).length > 0 ? null : error.message };
  }
}
