import type { ReactNode } from 'react';
import styles from './Chip.module.css';

export type ChipProps = { active?: boolean; count?: ReactNode; onClick?: () => void; children: ReactNode };

/** Toggle chip with an optional mono count (routes status filter). */
export function Chip({ active, count, onClick, children }: ChipProps) {
  return (
    <button type="button" aria-pressed={!!active} className={[styles.chip, active && styles.active].filter(Boolean).join(' ')} onClick={onClick}>
      {children}
      {count !== undefined && <span className={styles.count}>{count}</span>}
    </button>
  );
}
