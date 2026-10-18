import type { ReactNode } from 'react';
import styles from './StepHeading.module.css';

export type StepHeadingProps = { title: string; subtitle: string; actions?: ReactNode };

/** Step editor title (18px) with its one-line purpose and optional right-side controls. */
export function StepHeading({ title, subtitle, actions }: StepHeadingProps) {
  return (
    <div className={styles.heading}>
      <div className={styles.text}>
        <div className={styles.title}>{title}</div>
        <div className={styles.subtitle}>{subtitle}</div>
      </div>
      {actions}
    </div>
  );
}
