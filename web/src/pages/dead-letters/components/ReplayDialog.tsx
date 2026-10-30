import { useState } from 'react';
import { Button, Field, Modal, ModalWidth, TextArea } from '../../../components/ui';
import type { DeadLetter, DeadLetterOriginal } from '../../../domain';
import { actions } from '../../../store';
import { ReplayDraft, ReplayPhase } from '../model/ReplayDraft';
import { DiffList, DryRunResultView } from './DryRunResults';
import { RecordEditor } from './RecordEditor';
import styles from './ReplayDialog.module.css';

export type ReplayDialogProps = { deadLetter: DeadLetter; original: DeadLetterOriginal | null; onClose: () => void; onReplayed: (id: string) => void };

/** Edit-then-replay of one dead letter: edit, dry-run through the active version, then replay (or request approval). */
export function ReplayDialog({ deadLetter, original, onClose, onReplayed }: ReplayDialogProps) {
  const [draft, setDraft] = useState(() => ReplayDraft.of(original));
  const send = async (dryRun: boolean) => {
    const from = draft.phase;
    setDraft((d) => d.busy());
    const result = await actions.deadLetters.replay(deadLetter, draft.edit, { dryRun, note: draft.note.trim() });
    setDraft((d) => d.answered(result, from));
    if (result && !dryRun && !result.report?.failed.length) onReplayed(deadLetter.id);
  };
  const finished = draft.phase === ReplayPhase.Done || draft.phase === ReplayPhase.Pending;
  return (
    <Modal open onClose={onClose} title="Edit and replay" subtitle={`${deadLetter.kind} · ${deadLetter.step}`} width={ModalWidth.Lg} footer={<ReplayFooter draft={draft} finished={finished} onClose={onClose} onSend={send} />}>
      {draft.texts ? (
        <RecordEditor texts={draft.texts} errors={draft.errors} masked={!!original?.masked} disabled={finished || draft.phase === ReplayPhase.Busy} onType={(f, t) => setDraft((d) => d.type(f, t))} />
      ) : (
        <div className={styles.note}>{draft.blockedReason}</div>
      )}
      <div className={styles.section}>
        <div className={styles.label}>Changes</div>
        <DiffList diff={draft.result?.diff ?? draft.diff} />
      </div>
      <Field label="Note" hint="Kept in the audit log with the changed paths">
        <TextArea rows={2} value={draft.note} disabled={finished} onChange={(e) => setDraft((d) => d.withNote(e.target.value))} />
      </Field>
      <ReplayOutcome draft={draft} />
    </Modal>
  );
}

/** Preview of the dry run, or how the replay ended. */
function ReplayOutcome({ draft }: { draft: ReplayDraft }) {
  const r = draft.result;
  if (!r) return null;
  if (draft.phase === ReplayPhase.Pending) return <div className={styles.pending}>This route writes to a production destination: the replay waits until another approver approves it.</div>;
  if (draft.phase === ReplayPhase.Done) return <div className={styles.done}>{r.report?.failed.length ? `Failed again: ${r.report.failed[0].message}` : 'Replayed through the active version.'}</div>;
  return (
    <div className={styles.section}>
      <div className={styles.label}>Dry run{r.activeVersionNo ? ` · active version v${r.activeVersionNo}` : ''} · nothing was sent</div>
      {r.preview && <DryRunResultView result={r.preview} />}
    </div>
  );
}

type FooterProps = { draft: ReplayDraft; finished: boolean; onClose: () => void; onSend: (dryRun: boolean) => Promise<void> };

function ReplayFooter({ draft, finished, onClose, onSend }: FooterProps) {
  if (finished) return <Button variant="primary" onClick={onClose}>Close</Button>;
  const busy = draft.phase === ReplayPhase.Busy;
  return (
    <>
      <Button onClick={onClose}>Cancel</Button>
      <Button disabled={!draft.canSubmit} onClick={() => void onSend(true)}>
        {busy ? 'Working…' : 'Dry-run'}
      </Button>
      <Button variant="primary" disabled={!draft.canSubmit} onClick={() => void onSend(false)}>
        {draft.edit ? 'Replay edited event' : 'Replay'}
      </Button>
    </>
  );
}
