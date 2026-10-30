import { useCallback, useEffect, useState } from 'react';
import { Button, Card, CardHeader, Field, Format, Modal, TextArea } from '../../../components/ui';
import { useCan } from '../../../access';
import { DeadLetterJobs, Permission, type DeadLetterJob } from '../../../domain';
import { actions, useDeadLetterJobs, useNow } from '../../../store';
import styles from './ReplayApprovalsPanel.module.css';

/** A decision being made: approve or reject one job, with a note. */
type Decision = { job: DeadLetterJob; approve: boolean };

/** Replays waiting for a second person (strict 4-eyes on prod destinations); approvers decide, requesters only wait. */
export function ReplayApprovalsPanel() {
  const canApprove = useCan(Permission.DeploymentApprove);
  const [jobs, setJobs] = useState<DeadLetterJob[]>([]);
  const [decision, setDecision] = useState<Decision | null>(null);
  const load = useCallback(() => {
    actions.deadLetters.pendingApprovals().then(setJobs, () => setJobs([]));
  }, []);
  const requested = useDeadLetterJobs().filter((j) => DeadLetterJobs.awaitingApproval(j)).length;
  useEffect(load, [load, requested]);
  if (!jobs.length) return null;
  const decide = async (note: string) => {
    if (!decision) return;
    setDecision(null);
    if (await actions.deadLetters.decideJob(decision.job.id, decision.approve, note)) load();
  };
  return (
    <Card padding="md" className={styles.panel}>
      <CardHeader title="Replays waiting for approval" meta={`${jobs.length} pending · production destinations need a second person`} />
      {jobs.map((job) => (
        <PendingReplayRow key={job.id} job={job} canDecide={canApprove && !job.requestedByMe} onDecide={(approve) => setDecision({ job, approve })} />
      ))}
      <DecisionDialog decision={decision} onConfirm={decide} onClose={() => setDecision(null)} />
    </Card>
  );
}

function PendingReplayRow({ job, canDecide, onDecide }: { job: DeadLetterJob; canDecide: boolean; onDecide: (approve: boolean) => void }) {
  const now = useNow();
  const what = job.edited ? '1 edited event' : `${job.idsCount || job.total} events`;
  const who = job.requestedByMe ? 'you' : job.requestedBy || 'someone';
  return (
    <div className={styles.row}>
      <div className={styles.text}>
        <span>
          Replay {what} of {job.routeName || job.routeId}
          {job.ratePerSecond ? ` at ${job.ratePerSecond}/s` : ''}
        </span>
        <span className={styles.meta}>
          requested by {who} · {Format.relativeTime(job.createdAt, now)}
          {job.note ? ` · “${job.note}”` : ''}
        </span>
      </div>
      {canDecide ? (
        <div className={styles.actions}>
          <Button size="sm" onClick={() => onDecide(false)}>
            Reject
          </Button>
          <Button size="sm" variant="primary" onClick={() => onDecide(true)}>
            Approve
          </Button>
        </div>
      ) : (
        <span className={styles.meta}>{job.requestedByMe ? 'Waiting for another approver' : 'Needs an approver'}</span>
      )}
    </div>
  );
}

function DecisionDialog({ decision, onConfirm, onClose }: { decision: Decision | null; onConfirm: (note: string) => Promise<void>; onClose: () => void }) {
  const [note, setNote] = useState('');
  if (!decision) return null;
  const verb = decision.approve ? 'Approve' : 'Reject';
  const footer = (
    <>
      <Button onClick={onClose}>Cancel</Button>
      <Button variant={decision.approve ? 'primary' : 'danger'} onClick={() => void onConfirm(note.trim())}>
        {verb} replay
      </Button>
    </>
  );
  return (
    <Modal open onClose={onClose} title={`${verb} the replay of ${decision.job.routeName || decision.job.routeId}?`} footer={footer}>
      <Field label="Note" hint="Kept in the audit log">
        <TextArea rows={3} value={note} onChange={(e) => setNote(e.target.value)} />
      </Field>
    </Modal>
  );
}
