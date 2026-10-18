import { Card, CardPadding } from '../../../../components/ui';
import type { DryRunLine } from '../../model/ReviewModel';
import styles from './DryRunCard.module.css';

/** First outputs of a dry run over the samples: number, key, JSON value. */
export function DryRunCard({ lines }: { lines: DryRunLine[] }) {
  return (
    <Card padding={CardPadding.Md} className={styles.card}>
      <div className={styles.title}>Dry run · first outputs</div>
      {lines.map((l) => (
        <div key={l.label} className={styles.line}>
          <span className={styles.n}>{l.label}</span>
          <span className={styles.key}>{l.key}</span>
          <span className={styles.json}>{l.json}</span>
        </div>
      ))}
      {!lines.length && <div className={styles.none}>No outputs from the loaded samples yet.</div>}
    </Card>
  );
}
