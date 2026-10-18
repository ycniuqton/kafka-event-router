import { Button, Modal } from '../../../components/ui';
import styles from './RollbackDialog.module.css';

type RollbackDialogProps = { version?: number; nextVersion: number; onCancel: () => void; onConfirm: () => void };

/** Confirms "Roll back to this version": it is redeployed as a new version, history is kept. */
export function RollbackDialog({ version, nextVersion, onCancel, onConfirm }: RollbackDialogProps) {
  const footer = (
    <>
      <Button size="lg" onClick={onCancel}>Cancel</Button>
      <Button size="lg" variant="primary" onClick={onConfirm}>Roll back to v{version}</Button>
    </>
  );
  return (
    <Modal open={version !== undefined} onClose={onCancel} title={`Roll back to v${version}?`} footer={footer} hideClose>
      <p className={styles.text}>
        v{version} will be redeployed as v{nextVersion}. The route keeps running while it switches, and history is never lost — you can roll forward
        again at any time.
      </p>
    </Modal>
  );
}
