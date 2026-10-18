import { useMemo, useState } from 'react';
import { Button, Page, PageHeader, PageWidth } from '../../components/ui';
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
import styles from './ConnectionsPage.module.css';

/** Connections (spec §5): saved clusters with test, topic browser and the 4-step create / edit panel. */
export default function ConnectionsPage() {
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
    <div className={styles.root}>
      <Page width={PageWidth.Wide}>
        <PageHeader
          title="Connections"
          subtitle="Entered once, reused by every route."
          actions={
            <Can permission={Permission.ConnectionWrite}>
              <Button onClick={() => openPanel(ConnectionPanelMode.Create, null)}>+ New connection</Button>
            </Can>
          }
        />
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
      </Page>
      <ConnectionOverlays connections={connections} panel={panel} tests={tests} onClosePanel={() => setPanel(null)} onSaved={onSaved} />
      <DeleteConnectionModal connection={deleting} onClose={() => setDeleting(null)} />
    </div>
  );
}
