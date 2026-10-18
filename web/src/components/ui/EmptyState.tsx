import type { ReactNode } from 'react';
import styles from './EmptyState.module.css';

export type EmptyStateProps = { title: ReactNode; description?: ReactNode; action?: ReactNode; compact?: boolean };

/** Dashed box explaining what the screen is for and offering the next action (spec §12). */
export function EmptyState({ title, description, action, compact }: EmptyStateProps) {
  return (
    <div className={[styles.empty, compact && styles.compact].filter(Boolean).join(' ')}>
      <div className={styles.title}>{title}</div>
      {description && <div className={styles.description}>{description}</div>}
      {action && <div className={styles.action}>{action}</div>}
    </div>
  );
}
