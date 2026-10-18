import type { CSSProperties, ReactNode } from 'react';
import styles from './LibraryList.module.css';

/** A library row on the design's 12px-gap grid; clicking it (or Enter) opens the object. */
export function LibraryRow({ columns, onOpen, label, children }: { columns: string; onOpen?: () => void; label?: string; children: ReactNode }) {
  return (
    <div className={styles.row} data-clickable={!!onOpen} style={{ '--cols': columns } as CSSProperties} role={onOpen ? 'button' : undefined} tabIndex={onOpen ? 0 : undefined}
      aria-label={label} onClick={onOpen} onKeyDown={(e) => e.key === 'Enter' && onOpen?.()}>
      {children}
    </div>
  );
}
