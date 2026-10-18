import { useEffect, type RefObject } from 'react';
import { Keys, useShortcut } from '../../../../store';

/** Closes a popover on an outside click or Escape while it is open. */
export function useDismiss(ref: RefObject<HTMLElement | null>, open: boolean, onClose: () => void): void {
  useEffect(() => {
    if (!open) return undefined;
    const onPointer = (e: PointerEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener('pointerdown', onPointer);
    return () => document.removeEventListener('pointerdown', onPointer);
  }, [ref, open, onClose]);
  useShortcut({ key: Keys.Escape, allowInInputs: true, handler: onClose }, open);
}
