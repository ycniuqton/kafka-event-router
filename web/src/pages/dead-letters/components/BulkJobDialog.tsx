import { Button, Modal } from '../../../components/ui';
import { DeadLetterJobAction, DeadLetterReports } from '../../../domain';
import { BulkJobText } from '../model/DeadLetterTexts';
import { AtLeastOnceNotice } from './AtLeastOnceNotice';
import styles from './RetryReportDialog.module.css';

export type BulkJobDialogProps = {
  action: DeadLetterJobAction | null;
  routeName: string;
  unresolved: number;
  approximate: boolean;
  exactlyOnce?: boolean;
  onStart: (action: DeadLetterJobAction) => void;
  onRetryNow: () => void;
  onClose: () => void;
};

/** Over-cap prompt: start a background job (any size) or, for retry, handle the first 500 now. */
export function BulkJobDialog({ action, routeName, unresolved, approximate, exactlyOnce = false, onStart, onRetryNow, onClose }: BulkJobDialogProps) {
  if (!action) return null;
  const count = `${approximate ? '~' : ''}${unresolved}`;
  const retry = action === DeadLetterJobAction.Retry;
  const footer = (
    <>
      <Button onClick={onClose}>Cancel</Button>
      {retry && <Button onClick={onRetryNow}>Retry first {DeadLetterReports.syncCap} now</Button>}
      <Button variant={retry ? 'primary' : 'danger'} onClick={() => onStart(action)}>
        Start background {retry ? 'retry' : 'discard'}
      </Button>
    </>
  );
  return (
    <Modal open onClose={onClose} title={BulkJobText.title(action)} footer={footer}>
      <div className={styles.section}>{BulkJobText.body(action, routeName, count)}</div>
      <AtLeastOnceNotice show={retry && exactlyOnce} />
    </Modal>
  );
}
