import { Button, Card, CardPadding } from '../../../../components/ui';
import { DiffMarkTone, RouteDiffPresenter } from '../../../../domain';
import type { ReviewChecks } from '../../model/useReviewChecks';
import styles from './ChangesCard.module.css';

export type ChangesCardProps = { checks: ReviewChecks; changes: string[]; onSave: () => void };

/** Changes in this deploy: the server diff of the running version against the saved draft, by section. */
export function ChangesCard({ checks, changes, onSave }: ChangesCardProps) {
  const sections = checks.diff ? RouteDiffPresenter.sections(checks.diff) : [];
  return (
    <Card padding={CardPadding.Md} className={styles.card}>
      <div className={styles.title}>Changes in this deploy</div>
      {checks.firstDeploy && <div className={styles.none}>First version — everything is new.</div>}
      {!checks.firstDeploy && checks.diffStale && (
        <div className={styles.stale}>
          <span>Unsaved edits: {changes.join(', ') || 'changes'}.</span>
          <Button size="sm" onClick={onSave}>Save draft to compare</Button>
        </div>
      )}
      {checks.diffError && <div className={styles.none}>Diff unavailable: {checks.diffError.message}</div>}
      {sections.map((s) => (
        <div key={s.section} className={styles.section}>
          <div className={styles.sectionTitle}>{s.label}</div>
          {s.rows.map((r, i) => (
            <div key={i} className={styles.change} data-tone={DiffMarkTone[r.mark]}>
              {r.mark} {r.before && r.after && r.before !== r.after ? `${r.before} → ${r.after}` : (r.after ?? r.before)}
            </div>
          ))}
        </div>
      ))}
      {!checks.firstDeploy && !checks.diffStale && checks.diff && !sections.length && <div className={styles.none}>No changes since the running version.</div>}
    </Card>
  );
}
