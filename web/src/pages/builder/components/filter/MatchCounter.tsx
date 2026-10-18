import type { CSSProperties } from 'react';
import styles from './MatchCounter.module.css';

export type MatchCounterProps = {
  pass: number;
  total: number;
  pending?: boolean;
};

/** "Matches 9 of 24 sample events (37.5%)" from the preview service run, with a progress bar. */
export function MatchCounter({ pass, total, pending }: MatchCounterProps) {
  const pct = total ? (pass / total) * MatchCounterScale.percent : 0;
  return (
    <div className={styles.box} data-pending={pending || undefined}>
      <div className={styles.text}>
        {pending
          ? 'Running the filter on the samples…'
          : total
            ? `Matches ${pass} of ${total} sample events (${pct.toFixed(1)}%)`
            : 'No samples loaded — load samples to count matches'}
      </div>
      <div className={styles.track}>
        <div className={styles.fill} style={{ '--w': `${pct.toFixed(0)}%` } as CSSProperties} />
      </div>
    </div>
  );
}

/** Percentage scale of the counter. */
class MatchCounterScale {
  static readonly percent = 100;
}
