import { useState } from 'react';
import { Button, ButtonVariant, Modal } from '../../../components/ui';
import type { PasswordPolicy, PasswordResetDraft, TeamMember } from '../../../domain';
import { actions } from '../../../store';
import { NewUserField, UserAdmin, UserAdminText, type FieldMessages } from '../model/UserAdmin';
import { GeneratedPasswordView, PasswordSourceField } from './PasswordFields';
import styles from './UsersSection.module.css';

export type ResetPasswordDialogProps = { user: TeamMember; policy: PasswordPolicy; onClose: () => void; onReset: () => void };

/** Generate or set a new password; a generated one is shown once. */
export function ResetPasswordDialog({ user, policy, onClose, onReset }: ResetPasswordDialogProps) {
  const [draft, setDraft] = useState<PasswordResetDraft>(UserAdmin.blankReset());
  const [errors, setErrors] = useState<FieldMessages>({});
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [generated, setGenerated] = useState<string | null>(null);
  const submit = async () => {
    const local = UserAdmin.resetErrors(draft, policy);
    setErrors(local);
    if (Object.keys(local).length > 0) return;
    setBusy(true);
    try {
      const result = await UserAdmin.reset(user, draft);
      actions.toast.show(UserAdminText.ResetDone(user.name));
      onReset();
      if (result.generatedPassword) setGenerated(result.generatedPassword);
      else onClose();
    } catch (error) {
      const failure = UserAdmin.failure(error);
      setErrors(failure.fields);
      setMessage(failure.message);
    } finally {
      setBusy(false);
    }
  };
  const footer = generated ? (
    <div className={styles.footer}><Button onClick={onClose}>{UserAdminText.Done}</Button></div>
  ) : (
    <div className={styles.footer}>
      <Button variant={ButtonVariant.Secondary} onClick={onClose}>{UserAdminText.Cancel}</Button>
      <Button variant={ButtonVariant.Danger} disabled={busy} onClick={() => void submit()}>{UserAdminText.Reset}</Button>
    </div>
  );
  return (
    <Modal open onClose={onClose} title={`${UserAdminText.ResetTitle} · ${user.name}`} subtitle={UserAdminText.ResetSubtitle} footer={footer}>
      {generated ? <GeneratedPasswordView password={generated} /> : (
        <div className={styles.form}>
          {message && <div className={styles.error} role="alert">{message}</div>}
          <PasswordSourceField source={draft.passwordSource} password={draft.password} policy={policy} error={errors[NewUserField.Password]}
            onChange={(passwordSource, password) => setDraft({ passwordSource, password })} />
        </div>
      )}
    </Modal>
  );
}
