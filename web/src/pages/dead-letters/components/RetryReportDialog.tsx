import { Button, Modal, Mono } from '../../../components/ui';
import { DeadLetterSkipReasonLabel, type DeadLetter, type DeadLetterActionReport } from '../../../domain';
import { AtLeastOnceNotice } from './AtLeastOnceNotice';
import styles from './RetryReportDialog.module.css';

export type RetryReportDialogProps = { report: DeadLetterActionReport | null; rows: DeadLetter[]; onClose: () => void; onBackground?: () => void };

/** Per-outcome counts of a retry, the events that failed again with their new error (design §7.3) and the
 * at-least-once warning of exactly-once routes. */
export function RetryReportDialog({ report, rows, onClose, onBackground }: RetryReportDialogProps) {
  if (!report) return null;
  const label = (id: string) => {
    const row = rows.find((d) => d.id === id);
    return row ? `p${row.partition}@${row.offset}` : id;
  };
  const counts = [`${report.sent} sent`, `${report.filtered} filtered`, `${report.failed.length} failed`, `${report.skipped.length} skipped`];
  return (
    <Modal open onClose={onClose} title="Retry report" subtitle={counts.join(' · ')} footer={<Button onClick={onClose}>Close</Button>}>
      <AtLeastOnceNotice show={!!report.deliveredAtLeastOnce} />
      {report.failed.length > 0 && <div className={styles.section}>Failed again — fix the route, deploy, then retry</div>}
      <ul className={styles.list}>
        {report.failed.map((f) => (
          <li key={f.id} className={styles.item}>
            <Mono>{label(f.id)}</Mono>
            <span className={styles.error}>{f.message}</span>
          </li>
        ))}
      </ul>
      {report.skipped.length > 0 && <div className={styles.section}>Skipped</div>}
      <ul className={styles.list}>
        {report.skipped.map((s) => (
          <li key={s.id} className={styles.item}>
            <Mono>{label(s.id)}</Mono>
            <span>{DeadLetterSkipReasonLabel[s.reason] ?? s.reason}</span>
          </li>
        ))}
      </ul>
      {report.remaining > 0 && <div className={styles.section}>{report.remaining} more events match; retry again to continue.</div>}
      {report.remaining > 0 && onBackground && (
        <Button
          size="sm"
          onClick={() => {
            onClose();
            onBackground();
          }}
        >
          Retry the rest in the background
        </Button>
      )}
    </Modal>
  );
}
