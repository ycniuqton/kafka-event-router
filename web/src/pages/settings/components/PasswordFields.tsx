import { Button, ButtonSize, ButtonVariant, Field, SegmentedControl, TextInput } from '../../../components/ui';
import { PasswordSource, type PasswordPolicy } from '../../../domain';
import { actions } from '../../../store';
import { UserAdminText } from '../model/UserAdmin';
import styles from './UsersSection.module.css';

const SourceOptions = [
  { value: PasswordSource.Generate, label: UserAdminText.Generate },
  { value: PasswordSource.Manual, label: UserAdminText.Manual },
] as const;

export type PasswordSourceFieldProps = {
  source: PasswordSource;
  password: string;
  policy: PasswordPolicy;
  error?: string;
  onChange: (source: PasswordSource, password: string) => void;
};

/** "Generate" (server-made, shown once) or "Set password" with the policy's minimum length. */
export function PasswordSourceField({ source, password, policy, error, onChange }: PasswordSourceFieldProps) {
  const manual = source === PasswordSource.Manual;
  const hint = manual ? `At least ${policy.minLength} characters.` : UserAdminText.GenerateHint;
  return (
    <Field label={UserAdminText.Password} hint={hint} error={error}>
      <SegmentedControl options={SourceOptions} value={source} ariaLabel={UserAdminText.Password} onChange={(next) => onChange(next, password)} />
      {manual && (
        <TextInput type="password" block autoComplete="new-password" value={password} aria-label={UserAdminText.Password} onChange={(e) => onChange(source, e.target.value)} />
      )}
    </Field>
  );
}

/** The one-time generated password with a copy button and the "not shown again" warning. */
export function GeneratedPasswordView({ password }: { password: string }) {
  const copy = async () => {
    await navigator.clipboard?.writeText(password);
    actions.toast.show(UserAdminText.Copied);
  };
  return (
    <div className={styles.form}>
      <strong>{UserAdminText.GeneratedTitle}</strong>
      <div className={styles.secretBox}>
        <span className={styles.secretValue} data-testid="generated-password">{password}</span>
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Secondary} onClick={() => void copy()}>{UserAdminText.Copy}</Button>
      </div>
      <p className={styles.warning} role="alert">{UserAdminText.GeneratedBody}</p>
    </div>
  );
}
