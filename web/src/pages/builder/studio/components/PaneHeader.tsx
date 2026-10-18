import type { ReactNode } from 'react';
import styles from './PaneHeader.module.css';

type PaneHeaderProps = { title: ReactNode; grow?: boolean; raised?: boolean; children?: ReactNode };

/** Title row of a studio column (Input, Transform steps, Output). */
export function PaneHeader({ title, grow, raised, children }: PaneHeaderProps) {
  return (
    <div className={[styles.header, raised && styles.raised].filter(Boolean).join(' ')}>
      <span className={[styles.title, grow && styles.grow].filter(Boolean).join(' ')}>{title}</span>
      {children}
    </div>
  );
}
