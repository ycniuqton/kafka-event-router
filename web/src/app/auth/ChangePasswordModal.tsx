import { Button, ButtonVariant, Modal } from '../../components/ui';
import { actions } from '../../store';
import { PasswordMode } from './AuthConfigStore';
import { useAuthConfig } from './useAuthConfig';
import { ChangePasswordForm } from './ChangePasswordForm';

/** Self-service change copy. */
const ChangePasswordModalText = {
  Title: 'Change password',
  Subtitle: 'You stay signed in here; your other sessions are signed out.',
  Cancel: 'Cancel',
  Done: 'Password changed. Your other sessions were signed out.',
} as const;

/** User menu → "Change password": the same form as the forced screen, in a dialog. */
export function ChangePasswordModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const policy = PasswordMode.policy(useAuthConfig());
  const done = () => {
    actions.toast.show(ChangePasswordModalText.Done);
    onClose();
  };
  const cancel = (
    <Button type="button" variant={ButtonVariant.Secondary} onClick={onClose}>
      {ChangePasswordModalText.Cancel}
    </Button>
  );
  return (
    <Modal open={open} onClose={onClose} title={ChangePasswordModalText.Title} subtitle={ChangePasswordModalText.Subtitle}>
      {open && <ChangePasswordForm policy={policy} onChanged={done} actions={cancel} />}
    </Modal>
  );
}
