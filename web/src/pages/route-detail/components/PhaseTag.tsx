import type { Phase } from '../../../domain';
import { useSettings } from '../../../store';
import styles from './PhaseTag.module.css';

/** Delivery-phase tag (MVP/P2/P3), shown only when "Show phases" is on. */
export function PhaseTag({ phase }: { phase: Phase }) {
  const { showPhases } = useSettings();
  if (!showPhases) return null;
  return <span className={styles.tag}>{phase}</span>;
}
