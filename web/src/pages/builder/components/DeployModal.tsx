import { useState } from 'react';
import { Button, ButtonSize, ButtonVariant, Modal, TextArea } from '../../../components/ui';
import { DeployFailure, type DeployFailureView } from '../model/DeployFailure';
import styles from './DeployModal.module.css';

export type DeployModalProps = {
  open: boolean;
  title: string;
  onClose: () => void;
  onDeploy: (note: string, overwrite: boolean) => Promise<void>;
  onReload: () => void;
};

/** Asks for an optional change note, then deploys a new version; shows failures with Reload / Overwrite on 412. */
export function DeployModal({ open, title, onClose, onDeploy, onReload }: DeployModalProps) {
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const [failure, setFailure] = useState<DeployFailureView | null>(null);
  const deploy = async (overwrite: boolean) => {
    setBusy(true);
    setFailure(null);
    try {
      await onDeploy(note, overwrite);
    } catch (error) {
      setFailure(DeployFailure.of(error));
    } finally {
      setBusy(false);
    }
  };
  const footer = (
    <>
      <Button size={ButtonSize.Lg} onClick={onClose}>
        Cancel
      </Button>
      {failure?.conflict && <Button size={ButtonSize.Lg} onClick={onReload}>Reload</Button>}
      <Button variant={ButtonVariant.Primary} size={ButtonSize.Lg} disabled={busy} onClick={() => void deploy(!!failure?.conflict)}>
        {failure?.conflict ? 'Overwrite and deploy' : busy ? 'Deploying…' : 'Deploy'}
      </Button>
    </>
  );
  return (
    <Modal open={open} onClose={onClose} title={title} subtitle="Creates a new version. You can roll back in one click from History." hideClose footer={footer}>
      <TextArea
        className={styles.note}
        value={note}
        onChange={(e) => setNote(e.target.value)}
        placeholder="Change note (optional) — e.g. Mask email before sending to partner"
        autoFocus
      />
      {failure && <div className={styles.error}>{failure.message}</div>}
    </Modal>
  );
}
