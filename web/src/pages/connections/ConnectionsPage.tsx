import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { QueryParam } from '../../app/paths';
import { Button, Page, PageHeader, PageWidth, Tabs } from '../../components/ui';
import { Can } from '../../access';
import { Permission, type Connection } from '../../domain';
import { useConnections, useNow, useRoutes } from '../../store';
import { ConnectionFilters } from './components/ConnectionFilters';
import { ConnectionOverlays } from './components/ConnectionOverlays';
import { ConnectionsTable } from './components/ConnectionsTable';
import { DeleteConnectionModal } from './components/DeleteConnectionModal';
import { ConnectionPanelMode, type ConnectionPanel } from './model/ConnectionDrawer';
import { ConnectionFilter, type ConnectionFilterState } from './model/ConnectionFilter';
import { ConnectionRowModel } from './model/ConnectionRowModel';
import { useConnectionTestDrawer } from './model/useConnectionTestDrawer';
import { LookupSourcesTab } from './lookups/LookupSourcesTab';
import { ConnectionsTab, ConnectionsTabs } from './model/ConnectionsTab';
import styles from './ConnectionsPage.module.css';


/** Connections (spec §5): Kafka clusters and lookup sources, one tab each (`?tab=lookups`). */
export default function ConnectionsPage() {
  const [params, setParams] = useSearchParams();
  const tab = params.get(QueryParam.Tab) === ConnectionsTab.Lookups ? ConnectionsTab.Lookups : ConnectionsTab.Kafka;
  const select = (next: ConnectionsTab) => setParams(next === ConnectionsTab.Kafka ? {} : { [QueryParam.Tab]: next });
  return (
    <div className={styles.root}>
      <Page width={PageWidth.Wide}>
        <PageHeader title="Connections" subtitle="Entered once, reused by every route." />
        <Tabs items={ConnectionsTabs} value={tab} onChange={select} />
        <div className={styles.body}>{tab === ConnectionsTab.Kafka ? <KafkaClustersTab /> : <LookupSourcesTab />}</div>
      </Page>
    </div>
  );
}

/** Kafka clusters: saved clusters with test, topic browser and the 4-step create / edit panel. */
function KafkaClustersTab() {
  const connections = useConnections();
  const routes = useRoutes();
  const now = useNow();
  const tests = useConnectionTestDrawer(connections);
  const [panel, setPanel] = useState<ConnectionPanel | null>(null);
  const [deleting, setDeleting] = useState<Connection | null>(null);
  const [filter, setFilter] = useState<ConnectionFilterState>(ConnectionFilter.initial);
  const rows = useMemo(
    () => ConnectionFilter.apply(connections, filter).map((c) => ConnectionRowModel.build(c, routes, now)),
    [connections, filter, routes, now],
  );

  const openPanel = (mode: ConnectionPanelMode, connectionId: string | null) => {
    tests.closeTest();
    setPanel({ mode, connectionId });
  };
  const openTest = (id: string) => {
    setPanel(null);
    tests.openTest(id);
  };
  const onSaved = (id: string) => {
    setPanel(null);
    tests.showTest(id);
  };

  return (
    <>
      <div className={styles.toolbar}>
        <Can permission={Permission.ConnectionWrite}>
          <Button onClick={() => openPanel(ConnectionPanelMode.Create, null)}>+ New connection</Button>
        </Can>
      </div>
      {connections.length > 0 && <ConnectionFilters connections={connections} value={filter} onChange={setFilter} />}
      <ConnectionsTable
        rows={rows}
        filtered={ConnectionFilter.isActive(filter)}
        onCreate={() => openPanel(ConnectionPanelMode.Create, null)}
        onTest={openTest}
        onBrowse={(id) => openPanel(ConnectionPanelMode.Browse, id)}
        onEdit={(id) => openPanel(ConnectionPanelMode.Edit, id)}
        onDelete={setDeleting}
      />
      <ConnectionOverlays connections={connections} panel={panel} tests={tests} onClosePanel={() => setPanel(null)} onSaved={onSaved} />
      <DeleteConnectionModal connection={deleting} onClose={() => setDeleting(null)} />
    </>
  );
}
