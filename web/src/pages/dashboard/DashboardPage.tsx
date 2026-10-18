import { useMemo } from 'react';
import { services } from '../../api';
import { Page, PageHeader, PageWidth } from '../../components/ui';
import { PushState } from '../../domain';
import { useAppReady, useConnections, useNow, usePushState, useRoutes, useServiceQuery } from '../../store';
import { AttentionCard } from './components/AttentionCard';
import { FirstRunChecklist } from './components/FirstRunChecklist';
import { FlowMapCard } from './components/FlowMapCard';
import { HealthCards } from './components/HealthCards';
import { DashboardFreshness, DashboardSummary } from './model/DashboardSummary';
import { FirstRunChecklistManager } from './model/FirstRun';
import { useLiveRefresh } from './model/useRefreshEvery';
import styles from './DashboardPage.module.css';

/** Dashboard: answers "is everything flowing, and if not, where?" within 3 seconds (spec §4). */
export default function DashboardPage() {
  const ready = useAppReady();
  const routes = useRoutes();
  const connections = useConnections();
  const statusKey = routes.map((r) => r.status).join();
  const { data: loaded, reload } = useServiceQuery(() => DashboardFreshness.stamp(services.metrics.dashboard(), services.clock), [statusKey]);
  useLiveRefresh(reload);
  const now = useNow(DashboardFreshness.tickMs);
  const live = usePushState() === PushState.Live;
  const stats = loaded?.value;
  const summary = useMemo(() => DashboardSummary.of(routes, stats), [routes, stats]);
  const firstRun = useMemo(() => ({ routes, connections }), [routes, connections]);
  const showChecklist = ready && !FirstRunChecklistManager.isComplete(firstRun);
  return (
    <div className={styles.root}>
      <Page width={PageWidth.Wide} relaxed>
        <PageHeader title="Dashboard" subtitle={DashboardFreshness.subtitle(loaded?.at ?? null, now.getTime(), live)} />
        {showChecklist && <FirstRunChecklist state={firstRun} />}
        <HealthCards summary={summary} stats={stats} />
        <div className={styles.panels}>
          <FlowMapCard />
          <AttentionCard />
        </div>
      </Page>
    </div>
  );
}
