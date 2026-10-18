import { useEffect, type CSSProperties } from 'react';
import type { PanelValueAction, PanelValueActionKind } from '../../domain';
import { ValueActionLabels } from './ValueActionLabels';
import { Keys, useShortcut } from '../../store';
import styles from './ValueContextMenu.module.css';

/** Where the menu opens and for which value. */
export type ValueMenuTarget = { x: number; y: number; path: string; value: unknown };

export type ValueContextMenuProps = {
  target: ValueMenuTarget | null;
  kinds: readonly PanelValueActionKind[];
  onAction: (action: PanelValueAction) => void;
  onClose: () => void;
};

/** Small popover of value actions at the pointer; closes on Escape, outside click or after an action. */
export function ValueContextMenu({ target, kinds, onAction, onClose }: ValueContextMenuProps) {
  useShortcut({ key: Keys.Escape, allowInInputs: true, handler: onClose }, !!target);
  useEffect(() => {
    if (!target) return;
    const close = () => onClose();
    window.addEventListener('click', close);
    return () => window.removeEventListener('click', close);
  }, [target, onClose]);
  if (!target) return null;
  const position = { '--x': `${target.x}px`, '--y': `${target.y}px` } as CSSProperties;
  return (
    <div className={styles.menu} style={position} role="menu" onClick={(e) => e.stopPropagation()}>
      {kinds.map((kind) => (
        <button key={kind} type="button" role="menuitem" className={styles.item} onClick={() => {
            onAction({ kind, path: target.path, value: target.value });
            onClose();
          }}>
          {ValueActionLabels.labels[kind](target.path, target.value)}
        </button>
      ))}
    </div>
  );
}
