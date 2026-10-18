import type { ReactNode } from 'react';
import type { Tone } from '../../../domain';
import styles from './KpiTile.module.css';

/** Small stat tile: muted label above a 20px mono value. */
export function KpiTile({ label, tone, children }: { label: string; tone?: Tone; children: ReactNode }) {
  return (
    <div className={styles.tile}>
      <div className={styles.label}>{label}</div>
      <div className={styles.value} data-tone={tone}>
        {children}
      </div>
    </div>
  );
}
