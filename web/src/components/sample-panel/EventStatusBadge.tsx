import type { ProjectedState } from '../../domain';
import { EventStatusMeta } from './EventStatusMeta';
import styles from './EventStatusBadge.module.css';

export type EventStatusBadgeProps = { state: ProjectedState; withLabel?: boolean };

/** Status icon (✓ passed, ⊘ filtered, ⚠ error, ⊖ dropped) in the status tone, optionally labelled. */
export function EventStatusBadge({ state, withLabel }: EventStatusBadgeProps) {
  const meta = EventStatusMeta[state];
  return (
    <span className={styles.badge} data-tone={meta.tone} title={meta.label} aria-label={meta.label}>
      {meta.icon}
      {withLabel && <span className={styles.label}>{meta.label}</span>}
    </span>
  );
}
