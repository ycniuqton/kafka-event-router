import { useState } from 'react';
import { Button, ButtonVariant, Modal } from '../../../components/ui';
import type { TeamMember } from '../../../domain';
import { actions } from '../../../store';
import { UserAdmin, UserAdminText } from '../model/UserAdmin';
import styles from './UsersSection.module.css';

/** Confirms disabling a user (their sessions are revoked); errors stay in the dialog. */
export function DisableUserDialog({ user, onClose, onDone }: { user: TeamMember; onClose: () => void; onDone: () => void }) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const disable = async () => {
    setBusy(true);
    try {
      actions.toast.show(await UserAdmin.setDisabled(user, true));
      onDone();
      onClose();
    } catch (e) {
      setError(UserAdmin.failure(e).message);
      setBusy(false);
    }
  };
  const footer = (
    <div className={styles.footer}>
      <Button variant={ButtonVariant.Secondary} onClick={onClose}>{UserAdminText.Cancel}</Button>
      <Button variant={ButtonVariant.Danger} disabled={busy} onClick={() => void disable()}>{UserAdminText.Disable}</Button>
    </div>
  );
  return (
    <Modal open onClose={onClose} title={UserAdminText.DisableTitle} subtitle={user.name} footer={footer}>
      <p className={styles.note}>{UserAdminText.DisableBody}</p>
      {error && <div className={styles.error} role="alert">{error}</div>}
    </Modal>
  );
}
