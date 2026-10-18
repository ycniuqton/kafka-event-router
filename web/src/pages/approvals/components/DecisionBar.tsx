import { useState } from 'react';
import { services } from '../../../api';
import { Button, ButtonVariant, Card, TextArea } from '../../../components/ui';
import type { ApprovalDetail } from '../../../domain';
import { actions } from '../../../store';
import { DecisionKind, DecisionModel } from '../model/DecisionModel';
import styles from '../ApprovalReviewPage.module.css';

export type DecisionBarProps = { detail: ApprovalDetail; onDecided: () => void };

/** Approve (optional note) / Reject (note required) / Withdraw (requester); the server decides who may. */
export function DecisionBar({ detail, onDecided }: DecisionBarProps) {
  const [note, setNote] = useState('');
  const [busy, setBusy] = useState(false);
  const view = DecisionModel.view(detail, note);
  const decide = async (kind: DecisionKind) => {
    setBusy(true);
    try {
      const a = detail.approval;
      if (kind === DecisionKind.Approve) await services.approvals.approve(a, note);
      else if (kind === DecisionKind.Reject) await services.approvals.reject(a, note);
      else await services.approvals.withdraw(a);
      actions.toast.show(DecisionModel.done(kind));
      setNote('');
      onDecided();
    } catch (error) {
      actions.toast.show((error as Error).message);
      onDecided();
    } finally {
      setBusy(false);
    }
  };
  return (
    <Card padding="md" className={styles.decision}>
      {view.notice && <div className={styles.notice}>{view.notice}</div>}
      {(view.canApprove || view.canReject) && (
        <TextArea value={note} rows={2} placeholder="Note for the requester (required to reject)" aria-label="Decision note" onChange={(e) => setNote(e.target.value)} />
      )}
      <div className={styles.buttons}>
        {view.canWithdraw && (
          <Button disabled={busy} onClick={() => void decide(DecisionKind.Withdraw)}>
            Withdraw request
          </Button>
        )}
        {detail.canDecide && (
          <Button variant={ButtonVariant.Danger} disabled={busy || !view.canReject} onClick={() => void decide(DecisionKind.Reject)}>
            Reject
          </Button>
        )}
        {detail.canDecide && (
          <Button variant={ButtonVariant.Primary} disabled={busy || !view.canApprove} onClick={() => void decide(DecisionKind.Approve)}>
            Approve & deploy
          </Button>
        )}
      </div>
    </Card>
  );
}
