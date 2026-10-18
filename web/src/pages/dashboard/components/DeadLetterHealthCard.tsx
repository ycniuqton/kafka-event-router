import { useNavigate } from 'react-router-dom';
import { AppPaths } from '../../../app/paths';
import { Format } from '../../../components/ui';
import { Tone, type DashboardStats } from '../../../domain';
import { useDeadLetterCount } from '../../../store';
import { MetricsUnavailable } from '../model/DashboardSummary';
import { KpiCard } from './KpiCard';
import styles from './DeadLetterHealthCard.module.css';

/** Dead-letter events in the last 24 h from the server summary; the whole card opens the Dead letters screen. */
export function DeadLetterHealthCard({ stats }: { stats: DashboardStats | undefined }) {
  const navigate = useNavigate();
  const unresolved = useDeadLetterCount();
  const unavailable = stats?.metricsAvailable === false;
  const count = stats?.deadLetters24h ?? unresolved;
  const value = unavailable ? MetricsUnavailable.value : Format.number(count);
  const footer = <span className={styles.link}>Open dead letters →</span>;
  return (
    <KpiCard
      label="Dead-letter events · 24 h"
      value={value}
      valueTone={unavailable ? Tone.Muted : undefined}
      footer={footer}
      onClick={() => navigate(AppPaths.deadLetters())}
    />
  );
}
