import { useMemo } from 'react';
import { services } from '../../../api';
import { useIntentRunner } from '../../../app/useIntentRunner';
import { Card } from '../../../components/ui';
import { useActivity, useNow, useRoutes, useServiceQuery } from '../../../store';
import { AttentionView } from '../model/AttentionView';
import { DashboardLimits } from '../model/DashboardSummary';
import { useLiveRefresh } from '../model/useRefreshEvery';
import { ActivityRow } from './ActivityRow';
import { AttentionRow } from './AttentionRow';
import styles from './AttentionCard.module.css';

/** "Needs attention" rows with one-click actions, followed by the recent-activity feed. */
export function AttentionCard() {
  const run = useIntentRunner();
  const routes = useRoutes();
  const stored = useActivity();
  const now = useNow();
  const { data: attention, reload } = useServiceQuery(() => services.alerts.attention(), []);
  useLiveRefresh(reload);
  const { data: activity, reload: reloadActivity } = useServiceQuery(() => services.activity.list(), []);
  useLiveRefresh(reloadActivity);
  const recent = useMemo(() => (activity ?? stored).slice(0, DashboardLimits.recentActivity), [activity, stored]);
  return (
    <Card>
      <div className={styles.heading}>Needs attention</div>
      {attention?.map((item) => (
        <AttentionRow key={item.id} item={item} actionLabel={AttentionView.actionLabel(item, routes)} onAct={() => run(item.intent)} />
      ))}
      {attention?.length === 0 && <div className={styles.clear}>All routes are flowing. Nothing needs attention.</div>}
      <div className={styles.subheading}>Recent activity</div>
      {recent.map((a) => (
        <ActivityRow key={a.id} activity={a} now={now} />
      ))}
      {!recent.length && <div className={styles.none}>No changes yet. Deploys, edits, pauses and rollbacks appear here.</div>}
      <div className={styles.end} />
    </Card>
  );
}
