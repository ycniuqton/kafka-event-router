import { useState } from 'react';
import { Button, Field, Modal, TextArea } from '../../../components/ui';
import type { DeadLetterApprovalRequest } from '../../../domain';
import styles from './ReplayDialog.module.css';

export type ApprovalRequestDialogProps = { requests: DeadLetterApprovalRequest[]; onRequest: (note: string) => Promise<void>; onClose: () => void };

/** A retry the strict 4-eyes gate stopped: request an approved replay job for those events instead. */
export function ApprovalRequestDialog({ requests, onRequest, onClose }: ApprovalRequestDialogProps) {
  const [note, setNote] = useState('');
  if (!requests.length) return null;
  const count = requests.reduce((n, r) => n + r.ids.length, 0);
  const footer = (
    <>
      <Button onClick={onClose}>Cancel</Button>
      <Button variant="primary" onClick={() => void onRequest(note.trim())}>
        Request approval
      </Button>
    </>
  );
  return (
    <Modal open onClose={onClose} title="Replay needs approval" footer={footer}>
      <div className={styles.note}>
        {count === 1 ? 'This event goes' : `${count} events go`} to a production destination. Another approver has to approve the replay before anything is sent.
      </div>
      <Field label="Note for the approver">
        <TextArea rows={3} value={note} onChange={(e) => setNote(e.target.value)} />
      </Field>
    </Modal>
  );
}
