import type { DashboardStats } from '../../../domain';
import type { HealthSummary } from '../model/DashboardSummary';
import { DeadLetterHealthCard } from './DeadLetterHealthCard';
import { EventsHealthCard } from './EventsHealthCard';
import { LagHealthCard } from './LagHealthCard';
import { RoutesHealthCard } from './RoutesHealthCard';
import styles from './HealthCards.module.css';

/** The four health-summary cards answering "is everything flowing?" (spec §4). */
export function HealthCards({ summary, stats }: { summary: HealthSummary; stats: DashboardStats | undefined }) {
  return (
    <div className={styles.grid}>
      <RoutesHealthCard summary={summary} />
      <EventsHealthCard stats={stats} />
      <LagHealthCard summary={summary} />
      <DeadLetterHealthCard stats={stats} />
    </div>
  );
}
