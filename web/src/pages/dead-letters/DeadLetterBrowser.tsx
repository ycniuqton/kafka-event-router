import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { DeadLetterDelivery, DeadLetterJobAction } from '../../domain';
import { useDeadLetterSummary, useRoutes } from '../../store';
import { BulkJobDialog } from './components/BulkJobDialog';
import { DeadLetterDetail } from './components/DeadLetterDetail';
import { DeadLetterEmptyState } from './components/DeadLetterEmptyState';
import { DeadLetterJobPanel } from './components/DeadLetterJobPanel';
import { DeadLetterList } from './components/DeadLetterList';
import { DeadLetterToolbar } from './components/DeadLetterToolbar';
import { RetryReportDialog } from './components/RetryReportDialog';
import { DeadLetterExportTarget, DeadLetterRouteNames, DeadLetterViewCounts } from './model/DeadLetterBrowserModel';
import { DeadLetterQuery, DeadLetterViewParams } from './model/DeadLetterQuery';
import { useDeadLetterBrowser } from './model/useDeadLetterBrowser';
import { useDeadLetterCommands } from './model/useDeadLetterCommands';
import styles from './DeadLetterBrowser.module.css';

export type DeadLetterBrowserProps = { fixedRouteId?: string };

/** List / detail split over the real DLQ scan, shared by the Dead letters screen and the route's tab. */
export function DeadLetterBrowser({ fixedRouteId }: DeadLetterBrowserProps) {
  const [params, setParams] = useSearchParams();
  const view = useMemo(() => DeadLetterViewParams.parse(params, fixedRouteId), [params, fixedRouteId]);
  const model = useDeadLetterBrowser(view, params.get(DeadLetterQuery.Selected));
  const summary = useDeadLetterSummary();
  const routes = useRoutes();
  const names = useMemo(() => new DeadLetterRouteNames(routes, summary), [routes, summary]);
  const commands = useDeadLetterCommands();
  const [bulk, setBulk] = useState<DeadLetterJobAction | null>(null);
  const { state } = model;
  const update = (patch: Record<string, string | null>) =>
    setParams((prev) => {
      const next = new URLSearchParams(prev);
      Object.entries(patch).forEach(([key, value]) => (value ? next.set(key, value) : next.delete(key)));
      next.delete(DeadLetterQuery.Selected);
      return next;
    });
  const setParam = (key: string, value: string | null) => update({ [key]: value });
  const clear = () => update(DeadLetterViewParams.cleared(fixedRouteId));
  const unresolved = DeadLetterViewCounts.unresolved(summary, view);
  const target = DeadLetterExportTarget.of(view, state.checkedRows, model.filter.current);
  const toolbar = (
    <DeadLetterToolbar
      model={model}
      view={view}
      fixedRouteId={fixedRouteId}
      unresolved={unresolved}
      approximate={summary.approximate}
      kinds={DeadLetterViewCounts.kinds(summary, view)}
      names={names}
      onParam={setParam}
      onExport={target ? () => void commands.exportJson(target.routeId, target.selection, model.format) : null}
      onBulk={setBulk}
    />
  );
  const startJob = (action: DeadLetterJobAction) => {
    setBulk(null);
    if (view.routeId) void model.startJob(view.routeId, action);
  };
  const retryNow = () => {
    setBulk(null);
    if (view.routeId) void model.retryAll(view.routeId);
  };
  const empty = state.loading ? null : <DeadLetterEmptyState filtered={DeadLetterViewParams.isFiltered(view, fixedRouteId)} error={state.error} onClear={clear} onRetry={model.refresh} />;
  const selected = state.selected;
  return (
    <div className={styles.layout}>
      <DeadLetterList model={model} names={names} toolbar={<>{toolbar}<DeadLetterJobPanel routeId={fixedRouteId} /></>} empty={empty} />
      {selected && <DeadLetterDetail deadLetter={selected} routeName={names.of(selected.routeId, selected.routeName)} model={model} showAllLink={!!fixedRouteId} />}
      <RetryReportDialog report={model.report} rows={state.items} onClose={model.closeReport} onBackground={view.routeId ? () => setBulk(DeadLetterJobAction.Retry) : undefined} />
      <BulkJobDialog
        action={bulk}
        routeName={view.routeId ? names.of(view.routeId) : ''}
        unresolved={unresolved}
        approximate={summary.approximate}
        exactlyOnce={DeadLetterDelivery.exactlyOnce(state.items.filter((d) => d.routeId === view.routeId))}
        onStart={startJob}
        onRetryNow={retryNow}
        onClose={() => setBulk(null)}
      />
    </div>
  );
}
