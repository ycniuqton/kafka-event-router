import type { ReactNode } from 'react';
import { RouteStatusMeta, type RouteStatus, type Tone } from '../../domain';
import styles from './StatusBadge.module.css';

export type ToneBadgeProps = { tone: Tone; children: ReactNode; dot?: boolean };

/** Tinted pill in any tone (16% tone background, tone text). */
export function ToneBadge({ tone, children, dot = true }: ToneBadgeProps) {
  return (
    <span className={styles.badge} data-tone={tone}>
      {dot && <span className={styles.dot} />}
      {children}
    </span>
  );
}

export type StatusBadgeProps = { status: RouteStatus; dot?: boolean };

/** Route status badge: green Running, grey Paused, red Error, blue Draft. */
export function StatusBadge({ status, dot = true }: StatusBadgeProps) {
  const meta = RouteStatusMeta[status];
  return (
    <ToneBadge tone={meta.tone} dot={dot}>
      {meta.label}
    </ToneBadge>
  );
}
