import { useEffect, useState } from 'react';
import { useNavigate, useParams, useSearchParams } from 'react-router-dom';
import { AppPaths, QueryParam } from '../../app/paths';
import { Button, EmptyState, Page, PageWidth, Tabs } from '../../components/ui';
import type { Route, RouteDetailTab } from '../../domain';
import { actions, useAppReady, useDeadLetterCount, useRoute } from '../../store';
import { DetailHeader } from './components/DetailHeader';
import { RegisteredHeaderActions } from './components/header/RegisteredHeaderActions';
import { RegisteredTabs } from './components/RegisteredTabs';
import { MonitoringHeaderActions } from './header/MonitoringHeaderActions';
import { DetailTabs } from './model/DetailTabs';
import styles from './RouteDetailPage.module.css';

/** Route detail (spec §10–11): header actions and the registered tabs via `?tab=`; re-reads the route on open. */
export default function RouteDetailPage() {
  const { id } = useParams();
  const route = useRoute(id);
  const ready = useAppReady();
  const navigate = useNavigate();
  const [checked, setChecked] = useState(false);
  useEffect(() => {
    if (!id) return;
    void actions.routes
      .refresh(id)
      .catch(() => undefined)
      .finally(() => setChecked(true));
  }, [id]);
  if (!route) {
    return (
      <Page width={PageWidth.Wide}>
        {ready && checked && (
          <EmptyState
            title="Route not found"
            description="It may have been archived or the link is wrong."
            action={<Button variant="primary" onClick={() => navigate(AppPaths.routes())}>Back to routes</Button>}
          />
        )}
      </Page>
    );
  }
  return <RouteDetail route={route} />;
}

/** Detail body for an existing route; remounts per route id so tab-local state resets. */
function RouteDetail({ route }: { route: Route }) {
  const [params, setParams] = useSearchParams();
  const deadLetters = useDeadLetterCount(route.id);
  const tab = RegisteredTabs.ready && RegisteredHeaderActions.ready && MonitoringHeaderActions.ready ? DetailTabs.parse(params.get(QueryParam.Tab)) : null;
  const selectTab = (next: RouteDetailTab) => setParams({ [QueryParam.Tab]: next }, { replace: true });
  const entry = tab ? DetailTabs.find(tab) : undefined;
  const props = { route, deadLetters };
  return (
    <div className={styles.root}>
      <Page width={PageWidth.Wide}>
        <DetailHeader route={route} />
        {tab && <Tabs items={DetailTabs.items(props)} value={tab} onChange={selectTab} />}
        <div key={route.id} className={styles.body}>
          {entry && <entry.Component {...props} />}
        </div>
      </Page>
    </div>
  );
}
