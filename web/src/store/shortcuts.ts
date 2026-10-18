import { useEffect, useRef } from 'react';

/** Key values used by app shortcuts. */
export const Keys = { Escape: 'Escape', Enter: 'Enter', Slash: '/', N: 'n', S: 's' } as const;

/** A keyboard shortcut. `mod` means Ctrl (or ⌘ on macOS) must be held. */
export type ShortcutSpec = {
  key: string;
  mod?: boolean;
  /** Fire even while focus is in an input, textarea or select (default: false, except with `mod`). */
  allowInInputs?: boolean;
  handler: (event: KeyboardEvent) => void;
};

const typingTags = new Set(['INPUT', 'TEXTAREA', 'SELECT']);

/**
 * One window-level keydown listener dispatching to registered shortcuts.
 * The most recently registered matching shortcut wins (e.g. an open modal's Escape).
 */
export class ShortcutRegistry {
  private readonly stack: ShortcutSpec[] = [];

  constructor() {
    window.addEventListener('keydown', (e) => this.dispatch(e));
  }

  register(spec: ShortcutSpec): () => void {
    this.stack.push(spec);
    return () => {
      const index = this.stack.lastIndexOf(spec);
      if (index >= 0) this.stack.splice(index, 1);
    };
  }

  private dispatch(event: KeyboardEvent): void {
    const typing = typingTags.has((event.target as HTMLElement | null)?.tagName ?? '') || (event.target as HTMLElement | null)?.isContentEditable;
    const match = [...this.stack].reverse().find((spec) => ShortcutRegistry.matches(spec, event, !!typing));
    if (!match) return;
    event.preventDefault();
    match.handler(event);
  }

  private static matches(spec: ShortcutSpec, event: KeyboardEvent, typing: boolean): boolean {
    const mod = event.metaKey || event.ctrlKey;
    if (!!spec.mod !== mod) return false;
    if (typing && !spec.allowInInputs && !spec.mod) return false;
    return event.key.toLowerCase() === spec.key.toLowerCase();
  }
}

export const shortcuts = new ShortcutRegistry();

/** Registers a shortcut while the component is mounted and `enabled` is true. */
export function useShortcut(spec: ShortcutSpec, enabled = true): void {
  const handlerRef = useRef(spec.handler);
  useEffect(() => {
    handlerRef.current = spec.handler;
  });
  const { key, mod, allowInInputs } = spec;
  useEffect(() => {
    if (!enabled) return undefined;
    return shortcuts.register({ key, mod, allowInInputs, handler: (e) => handlerRef.current(e) });
  }, [key, mod, allowInInputs, enabled]);
}
