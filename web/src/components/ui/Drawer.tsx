import type { CSSProperties, ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { Keys, useShortcut } from '../../store/shortcuts';
import { DrawerWidth } from './variants';
import styles from './Overlay.module.css';

export type DrawerProps = {
  open: boolean;
  onClose: () => void;
  title: ReactNode;
  width?: number;
  footer?: ReactNode;
  children?: ReactNode;
};

/** Right-side panel (connection test, topic browser, create connection); Escape/scrim closes. */
export function Drawer({ open, onClose, title, width = DrawerWidth.Default, footer, children }: DrawerProps) {
  useShortcut({ key: Keys.Escape, allowInInputs: true, handler: onClose }, open);
  if (!open) return null;
  return createPortal(
    <>
      <div className={styles.drawerScrim} onClick={onClose} />
      <aside className={styles.drawer} role="dialog" aria-modal="true" style={{ '--drawer-w': `${width}px` } as CSSProperties}>
        <div className={styles.drawerHead}>
          <span className={styles.drawerTitle}>{title}</span>
          <button type="button" className={styles.close} onClick={onClose} aria-label="Close">
            ×
          </button>
        </div>
        <div className={styles.drawerBody}>{children}</div>
        {footer && <div className={styles.drawerFooter}>{footer}</div>}
      </aside>
    </>,
    document.body,
  );
}
