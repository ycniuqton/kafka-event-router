import { useMemo, useState, type FormEvent, type ReactNode } from 'react';
import type { SessionUser } from '../../api/http/core';
import { Button, ButtonVariant, Field, TextInput } from '../../components/ui';
import { PasswordChangeField, PasswordRules, type PasswordChangeDraft, type PasswordPolicy } from '../../domain';
import { PasswordChange } from './PasswordChange';
import { SessionServices } from './SessionServices';
import styles from './Auth.module.css';

/** Change-password form copy. */
const ChangePasswordText = {
  Current: 'Current password',
  Next: 'New password',
  Confirm: 'Confirm new password',
  Submit: 'Change password',
  Saving: 'Changing…',
  Hint: (policy: PasswordPolicy) => `At least ${policy.minLength} characters. Other sessions are signed out.`,
} as const;

export type ChangePasswordFormProps = { policy: PasswordPolicy; onChanged: (user: SessionUser) => void; actions?: ReactNode };

/** Current / new / confirm; checks the policy locally, then shows the server's field problems in place. */
export function ChangePasswordForm({ policy, onChanged, actions }: ChangePasswordFormProps) {
  const change = useMemo(() => new PasswordChange(SessionServices.current(), policy), [policy]);
  const [draft, setDraft] = useState<PasswordChangeDraft>(PasswordRules.blankChange());
  const [busy, setBusy] = useState(false);
  const [errors, setErrors] = useState<Partial<Record<PasswordChangeField, string>>>({});
  const [message, setMessage] = useState<string | null>(null);
  const set = (field: PasswordChangeField, value: string) => setDraft({ ...draft, [field]: value });
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    setBusy(true);
    const outcome = await change.submit(draft);
    setBusy(false);
    setErrors(outcome.errors);
    setMessage(outcome.message);
    if (outcome.user) onChanged(outcome.user);
  };
  return (
    <form className={styles.form} onSubmit={(e) => void submit(e)}>
      {message && <div className={styles.error} role="alert">{message}</div>}
      <PasswordField label={ChangePasswordText.Current} autoComplete="current-password" value={draft.current_password} error={errors.current_password}
        onChange={(v) => set(PasswordChangeField.Current, v)} />
      <PasswordField label={ChangePasswordText.Next} autoComplete="new-password" value={draft.new_password} error={errors.new_password}
        hint={ChangePasswordText.Hint(policy)} onChange={(v) => set(PasswordChangeField.Next, v)} />
      <PasswordField label={ChangePasswordText.Confirm} autoComplete="new-password" value={draft.confirm_password} error={errors.confirm_password}
        onChange={(v) => set(PasswordChangeField.Confirm, v)} />
      <div className={styles.actions}>
        {actions}
        <Button type="submit" variant={ButtonVariant.Primary} disabled={busy}>
          {busy ? ChangePasswordText.Saving : ChangePasswordText.Submit}
        </Button>
      </div>
    </form>
  );
}

type PasswordFieldProps = { label: string; autoComplete: string; value: string; error?: string; hint?: string; onChange: (value: string) => void };

/** One masked password input with its label, hint and error. */
function PasswordField({ label, autoComplete, value, error, hint, onChange }: PasswordFieldProps) {
  return (
    <Field label={label} hint={hint} error={error}>
      <TextInput type="password" block autoComplete={autoComplete} value={value} onChange={(e) => onChange(e.target.value)} />
    </Field>
  );
}
