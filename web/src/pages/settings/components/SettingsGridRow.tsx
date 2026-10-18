import type { CSSProperties, ReactNode } from 'react';
import styles from './SettingsSection.module.css';

/** A grid row inside a settings card. */
export function SettingsGridRow({ columns, children }: { columns: string; children: ReactNode }) {
  return (
    <div className={styles.row} style={{ '--cols': columns } as CSSProperties}>
      {children}
    </div>
  );
}
