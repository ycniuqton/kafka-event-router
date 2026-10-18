import type { CSSProperties, ReactNode } from 'react';
import { createPortal } from 'react-dom';
import { Keys, useShortcut } from '../../store/shortcuts';
import { ModalWidth } from './variants';
import styles from './Overlay.module.css';

export type ModalProps = {
  open: boolean;
  onClose: () => void;
  title?: ReactNode;
  subtitle?: ReactNode;
  width?: number;
  footer?: ReactNode;
  /** Hide the × button (e.g. confirm dialogs with Cancel). */
  hideClose?: boolean;
  children?: ReactNode;
};

/** Centered dialog over a dark scrim; closes on scrim click and Escape. */
export function Modal({ open, onClose, title, subtitle, width = ModalWidth.Sm, footer, hideClose, children }: ModalProps) {
  useShortcut({ key: Keys.Escape, allowInInputs: true, handler: onClose }, open);
  if (!open) return null;
  return createPortal(
    <div className={styles.modalScrim} onClick={onClose}>
      <div
        className={styles.modal}
        role="dialog"
        aria-modal="true"
        style={{ '--modal-w': `${width}px` } as CSSProperties}
        onClick={(e) => e.stopPropagation()}
      >
        {(title || !hideClose) && (
          <div className={styles.modalHead}>
            <div className={styles.modalTitles}>
              {title && <div className={styles.modalTitle}>{title}</div>}
              {subtitle && <div className={styles.modalSubtitle}>{subtitle}</div>}
            </div>
            {!hideClose && (
              <button type="button" className={styles.close} onClick={onClose} aria-label="Close">
                ×
              </button>
            )}
          </div>
        )}
        {children}
        {footer && <div className={styles.footer}>{footer}</div>}
      </div>
    </div>,
    document.body,
  );
}
