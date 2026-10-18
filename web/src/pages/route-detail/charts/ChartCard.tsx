import type { ReactNode } from 'react';
import { Card } from '../../../components/ui';
import type { ChartSeries } from './ChartScale';
import styles from './ChartCard.module.css';

type ChartCardProps = { title: ReactNode; legend?: Pick<ChartSeries, 'key' | 'label' | 'tone'>[]; children: ReactNode };

/** Chart panel: bold title, optional legend swatches, then the plot. */
export function ChartCard({ title, legend, children }: ChartCardProps) {
  return (
    <Card padding="md">
      <div className={styles.head}>
        <span className={styles.title}>{title}</span>
        {legend?.map((s) => (
          <span key={s.key} className={styles.legend} data-tone={s.tone}>
            <span className={s.tone ? styles.swatch : `${styles.swatch} ${styles.neutral}`} />
            {s.label}
          </span>
        ))}
      </div>
      {children}
    </Card>
  );
}
