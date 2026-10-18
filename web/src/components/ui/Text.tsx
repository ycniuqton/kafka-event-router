import type { ReactNode } from 'react';
import type { Tone } from '../../domain';
import { TextSize } from './variants';
import styles from './Text.module.css';

export type MonoProps = {
  children: ReactNode;
  size?: TextSize;
  tone?: Tone;
  medium?: boolean;
  truncate?: boolean;
  title?: string;
  className?: string;
};

/** Geist Mono text for topics, field paths, values, offsets and code (spec §12). */
export function Mono({ children, size = 'sm', tone, medium, truncate, title, className }: MonoProps) {
  const classes = [styles.mono, styles[size], tone && styles.toned, medium && styles.medium, truncate && styles.truncate, className]
    .filter(Boolean)
    .join(' ');
  return (
    <span className={classes} data-tone={tone} title={title}>
      {children}
    </span>
  );
}

/** Keyboard key hint, e.g. the "/" in the search box. */
export function Kbd({ children }: { children: ReactNode }) {
  return <kbd className={styles.kbd}>{children}</kbd>;
}

export type TooltipProps = { label: string; children: ReactNode; underline?: boolean };

/** Native-title tooltip; `underline` adds a dotted hint that more info exists. */
export function Tooltip({ label, children, underline }: TooltipProps) {
  return (
    <span title={label} className={underline ? styles.tip : styles.plain}>
      {children}
    </span>
  );
}
