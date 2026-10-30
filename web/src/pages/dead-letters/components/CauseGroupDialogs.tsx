import { useEffect, useState } from 'react';
import { Button, Field, Modal, ModalWidth, Mono, TextArea, TextInput } from '../../../components/ui';
import { DeadLetterJobAction, DryRunOutcome, DryRunOutcomeLabel, ReplayRate, type DeadLetterGroup, type DryRunReport } from '../../../domain';
import { actions } from '../../../store';
import { ReplayTargets, type ReplayJobTarget } from '../model/ReplayTargets';
import { DryRunResultView } from './DryRunResults';
import styles from './ReplayDialog.module.css';

/** Dry run of the newest events of one cause through the route's active version (nothing is sent). */
export function GroupDryRunDialog({ group, onClose, onReplay }: { group: DeadLetterGroup | null; onClose: () => void; onReplay: (g: DeadLetterGroup) => void }) {
  const [report, setReport] = useState<DryRunReport | null>(null);
  useEffect(() => {
    if (!group) return;
    let live = true;
    void actions.deadLetters.dryRun(group.routeId, { filter: { routeId: group.routeId, kinds: [], fingerprint: group.fingerprint } }).then((r) => live && setReport(r));
    return () => {
      live = false;
      setReport(null);
    };
  }, [group]);
  if (!group) return null;
  const counts = report ? Object.values(DryRunOutcome).map((o) => `${report.counts[o] ?? 0} ${DryRunOutcomeLabel[o].toLowerCase()}`).join(' · ') : 'Running…';
  const footer = (
    <>
      <Button onClick={onClose}>Close</Button>
      <Button variant="primary" onClick={() => onReplay(group)}>
        Replay group…
      </Button>
    </>
  );
  return (
    <Modal open onClose={onClose} title="Dry run of this cause" subtitle={report ? `Active version v${report.activeVersionNo} · ${report.sampled} newest events` : undefined} width={ModalWidth.Lg} footer={footer}>
      <div className={styles.section}>
        <Mono>{group.pattern}</Mono>
      </div>
      <div className={styles.note}>{counts}</div>
      <div className={styles.results}>{report?.results.map((r) => <DryRunResultView key={r.id} result={r} />)}</div>
    </Modal>
  );
}

/** Bulk replay as a background job throttled to a rate; progress shows in the job panel. */
export function ReplayGroupDialog({ group, onClose }: { group: DeadLetterGroup | null; onClose: () => void }) {
  return <ReplayJobDialog target={group ? ReplayTargets.group(group) : null} onClose={onClose} />;
}

export function ReplayJobDialog({ target, onClose }: { target: ReplayJobTarget | null; onClose: () => void }) {
  const [rate, setRate] = useState(String(ReplayRate.default));
  const [note, setNote] = useState('');
  if (!target) return null;
  const parsed = ReplayRate.parse(rate);
  const start = async () => {
    if (!parsed) return;
    const options = { ratePerSecond: parsed, note: note.trim(), ids: target.ids };
    if (await actions.deadLetters.startJob(target.routeId, DeadLetterJobAction.Retry, target.filter, options)) onClose();
  };
  const footer = (
    <>
      <Button onClick={onClose}>Cancel</Button>
      <Button variant="primary" disabled={!parsed} onClick={() => void start()}>
        Replay {target.count} events
      </Button>
    </>
  );
  return (
    <Modal open onClose={onClose} title={target.title} subtitle={target.subtitle} footer={footer}>
      <div className={styles.section}>
        <Mono>{target.detail}</Mono>
      </div>
      <Field label="Rate (events per second)" error={parsed ? undefined : `Use ${ReplayRate.min}–${ReplayRate.max}.`} hint={parsed ? `About ${ReplayRate.seconds(target.count, parsed)} s for ${target.count} events` : undefined}>
        <TextInput mono type="number" min={ReplayRate.min} max={ReplayRate.max} value={rate} onChange={(e) => setRate(e.target.value)} />
      </Field>
      <Field label="Note" hint="Shown to the approver when the destination is production, and kept in the audit log">
        <TextArea rows={2} value={note} onChange={(e) => setNote(e.target.value)} />
      </Field>
      <div className={styles.note}>Events run through the route's active version. Production destinations may need a second person's approval first.</div>
    </Modal>
  );
}
