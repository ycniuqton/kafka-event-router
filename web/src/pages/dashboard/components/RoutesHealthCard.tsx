import { useNavigate } from 'react-router-dom';
import { AppPaths } from '../../../app/paths';
import { RouteStatus, Tone } from '../../../domain';
import type { HealthSummary } from '../model/DashboardSummary';
import { KpiCard } from './KpiCard';
import styles from './RoutesHealthCard.module.css';

/** "42 running · 3 paused · 1 error" — the error count opens the routes list filtered to errors. */
export function RoutesHealthCard({ summary }: { summary: HealthSummary }) {
  const navigate = useNavigate();
  const value = (
    <>
      {summary.running}{' '}
      <span className={styles.running} data-tone={Tone.Ok}>
        running
      </span>
    </>
  );
  const footer = (
    <>
      <span>{summary.paused} paused</span>
      <button type="button" className={styles.errors} data-tone={Tone.Err} onClick={() => navigate(AppPaths.routes({ status: RouteStatus.Error }))}>
        {summary.errors} error
      </button>
    </>
  );
  return <KpiCard label="Routes" value={value} footer={footer} />;
}
