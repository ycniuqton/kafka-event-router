import type { ReactNode } from 'react';
import type { Tone } from '../../domain';
import styles from './HealthDot.module.css';

export type HealthDotProps = { tone: Tone; label?: ReactNode; pulse?: boolean; large?: boolean };

/** Coloured status dot with an optional label in the same tone (connections, alerts, attention). */
export function HealthDot({ tone, label, pulse, large }: HealthDotProps) {
  const dotClass = [styles.dot, large && styles.lg, pulse && styles.pulse].filter(Boolean).join(' ');
  return (
    <span className={styles.wrap} data-tone={tone}>
      <span className={dotClass} />
      {label}
    </span>
  );
}
