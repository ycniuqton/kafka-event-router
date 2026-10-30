import type { CSSProperties } from 'react';
import { DeadLetterGroups, type DeadLetterTrend } from '../../../domain';
import styles from './TrendBars.module.css';

/** Hourly counts of one cause over the window as mini bars (busiest hour = full height). */
export function TrendBars({ trend, tone }: { trend: DeadLetterTrend; tone: string }) {
  const total = trend.counts.reduce((n, c) => n + c, 0);
  return (
    <div className={styles.bars} data-tone={tone} role="img" aria-label={`${total} in the last ${trend.counts.length} hours`}>
      {DeadLetterGroups.bars(trend).map((h, i) => (
        <span key={i} className={styles.bar} style={{ '--h': `${h}%` } as CSSProperties} />
      ))}
    </div>
  );
}
