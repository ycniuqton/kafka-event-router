import { useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { Page, PageHeader, PageWidth } from '../../components/ui';
import { useNow, useRoutes } from '../../store';
import { BulkActionBar } from './components/BulkActionBar';
import { RoutesBody } from './components/RoutesBody';
import { RoutesDialogs } from './components/RoutesDialogs';
import { RoutesToolbar } from './components/RoutesToolbar';
import { RouteCommands } from './model/RouteCommands';
import { BulkRouteActions } from './model/BulkRouteActions';
import { NarrowViewport, RoutesView } from './model/RouteListModel';
import { RouteSelection } from './model/RouteSelection';
import { RoutesDialog } from './model/RoutesDialog';
import { useRoutesView } from './model/useRoutesView';
import { useMediaQuery } from './model/useMediaQuery';
import { useRouteListParams } from './model/useRouteListParams';
import { useRoutesListData } from './model/useRoutesListData';
import styles from './RoutesPage.module.css';

/** Routes list (spec §6): search, owner/status filters (archived too), bulk actions, import, remembered table/card view. */
export default function RoutesPage() {
  const routes = useRoutes();
  const now = useNow();
  const navigate = useNavigate();
  const params = useRouteListParams();
  const narrow = useMediaQuery(NarrowViewport.query);
  const [view, changeView] = useRoutesView();
  const [selection, setSelection] = useState(RouteSelection.empty);
  const [dialog, setDialog] = useState<RoutesDialog>(RoutesDialog.None);
  const [archiving, setArchiving] = useState<string | null>(null);
  const { rows, pool, counted, reloadArchived } = useRoutesListData(routes, params);
  const commands = useMemo(() => new RouteCommands(navigate, setArchiving, reloadArchived), [navigate, reloadArchived]);
  const bulk = new BulkRouteActions(selection, () => setSelection(RouteSelection.empty));
  return (
    <div className={styles.root}>
      <Page width={PageWidth.Wide}>
        <PageHeader title="Routes" subtitle="A route moves events from one topic to another, filtered and reshaped." />
        <RoutesToolbar
          routes={routes}
          counted={counted}
          params={params}
          view={view}
          onView={changeView}
          onImport={() => setDialog(RoutesDialog.Import)}
          bulk={selection.size > 0 && <BulkActionBar count={selection.size} bulk={bulk} onAddTag={() => setDialog(RoutesDialog.Tag)} />}
        />
        <RoutesBody
          rows={rows}
          hasRoutes={pool.length > 0}
          cards={narrow || view === RoutesView.Cards}
          selection={selection}
          onToggle={(id) => setSelection((s) => s.toggle(id))}
          commands={commands}
          now={now}
        />
      </Page>
      <RoutesDialogs
        open={dialog}
        archiving={routes.find((r) => r.id === archiving)}
        selected={selection.size}
        bulk={bulk}
        commands={commands}
        onClose={() => setDialog(RoutesDialog.None)}
        onArchiveDone={() => setArchiving(null)}
      />
    </div>
  );
}
