import { useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';
import { SegmentedControl } from '../../components/ui';
import { DeadLetterDelivery, DeadLetterJobAction, type DeadLetterGroup } from '../../domain';
import { useDeadLetterSummary, useRoutes } from '../../store';
import { ApprovalRequestDialog } from './components/ApprovalRequestDialog';
import { BulkJobDialog } from './components/BulkJobDialog';
import { CauseGroupList } from './components/CauseGroupList';
import { GroupDryRunDialog, ReplayGroupDialog } from './components/CauseGroupDialogs';
import { DeadLetterDetail } from './components/DeadLetterDetail';
import { DeadLetterEmptyState } from './components/DeadLetterEmptyState';
import { DeadLetterJobPanel } from './components/DeadLetterJobPanel';
import { DeadLetterList } from './components/DeadLetterList';
import { DeadLetterToolbar } from './components/DeadLetterToolbar';
import { RetryReportDialog } from './components/RetryReportDialog';
import { DeadLetterExportTarget, DeadLetterRouteNames, DeadLetterViewCounts } from './model/DeadLetterBrowserModel';
import { DeadLetterQuery, DeadLetterViewMode, DeadLetterViewModeOptions, DeadLetterViewParams, type DeadLetterView } from './model/DeadLetterQuery';
import { useCauseGroups } from './model/useCauseGroups';
import { useDeadLetterBrowser, type DeadLetterBrowserModel } from './model/useDeadLetterBrowser';
import { useDeadLetterCommands } from './model/useDeadLetterCommands';
import styles from './DeadLetterBrowser.module.css';

export type DeadLetterBrowserProps = { fixedRouteId?: string };

type Update = (patch: Record<string, string | null>) => void;

/** Events (list / detail split over the DLQ scan) or Causes (fingerprint groups), shared by the Dead letters screen and the route tab. */
export function DeadLetterBrowser({ fixedRouteId }: DeadLetterBrowserProps) {
  const [params, setParams] = useSearchParams();
  const view = useMemo(() => DeadLetterViewParams.parse(params, fixedRouteId), [params, fixedRouteId]);
  const model = useDeadLetterBrowser(view, params.get(DeadLetterQuery.Selected));
  const summary = useDeadLetterSummary();
  const routes = useRoutes();
  const names = useMemo(() => new DeadLetterRouteNames(routes, summary), [routes, summary]);
  const update: Update = (patch) =>
    setParams((prev) => {
      const next = new URLSearchParams(prev);
      Object.entries(patch).forEach(([key, value]) => (value ? next.set(key, value) : next.delete(key)));
      next.delete(DeadLetterQuery.Selected);
      return next;
    });
  const causes = view.mode === DeadLetterViewMode.Causes;
  return (
    <div className={styles.root}>
      <div className={styles.modes}>
        <SegmentedControl
          options={DeadLetterViewModeOptions}
          value={view.mode}
          ariaLabel="Dead-letter view"
          onChange={(mode) => update({ [DeadLetterQuery.View]: mode === DeadLetterViewMode.Causes ? mode : null })}
        />
        {view.fingerprint && <FingerprintFilter onClear={() => update({ [DeadLetterQuery.Fingerprint]: null })} />}
      </div>
      {causes ? (
        <CausesView view={view} names={names} fixedRouteId={fixedRouteId} model={model} update={update} />
      ) : (
        <EventsView view={view} names={names} fixedRouteId={fixedRouteId} model={model} update={update} />
      )}
      <ApprovalRequestDialog requests={model.approval} onRequest={model.requestApproval} onClose={model.closeApproval} />
    </div>
  );
}

function FingerprintFilter({ onClear }: { onClear: () => void }) {
  return (
    <span className={styles.fingerprint}>
      Showing one cause ·{' '}
      <button type="button" className={styles.link} onClick={onClear}>
        show all
      </button>
    </span>
  );
}

type ViewProps = { view: DeadLetterView; names: DeadLetterRouteNames; fixedRouteId?: string; model: DeadLetterBrowserModel; update: Update };

/** Causes: groups of the filter with drill-down, dry run and rate-limited group replay. */
function CausesView({ view, names, fixedRouteId, model, update }: ViewProps) {
  const groups = useCauseGroups(view, true);
  const [dryRun, setDryRun] = useState<DeadLetterGroup | null>(null);
  const [replay, setReplay] = useState<DeadLetterGroup | null>(null);
  const toolbar = useToolbar({ view, names, fixedRouteId, model, update }, null);
  const showEvents = (g: DeadLetterGroup) => update({ [DeadLetterQuery.View]: null, [DeadLetterQuery.Fingerprint]: g.fingerprint, ...(fixedRouteId ? {} : { [DeadLetterQuery.Route]: g.routeId }) });
  const openReplay = (g: DeadLetterGroup) => {
    setDryRun(null);
    setReplay(g);
  };
  return (
    <div className={styles.causes}>
      {toolbar}
      <DeadLetterJobPanel routeId={fixedRouteId} />
      <CauseGroupList groups={groups} names={names} showRoute={!fixedRouteId} onShowEvents={showEvents} onDryRun={setDryRun} onReplay={openReplay} />
      <GroupDryRunDialog group={dryRun} onClose={() => setDryRun(null)} onReplay={openReplay} />
      <ReplayGroupDialog key={replay?.fingerprint} group={replay} onClose={() => setReplay(null)} />
    </div>
  );
}

/** The toolbar shared by both views (filters, chips, bulk actions). */
function useToolbar({ view, names, fixedRouteId, model, update }: ViewProps, onBulk: ((a: DeadLetterJobAction) => void) | null) {
  const summary = useDeadLetterSummary();
  const commands = useDeadLetterCommands();
  const target = DeadLetterExportTarget.of(view, model.state.checkedRows, model.filter.current);
  return (
    <DeadLetterToolbar
      model={model}
      view={view}
      fixedRouteId={fixedRouteId}
      unresolved={DeadLetterViewCounts.unresolved(summary, view)}
      approximate={summary.approximate}
      kinds={DeadLetterViewCounts.kinds(summary, view)}
      classCounts={DeadLetterViewCounts.classes(summary, view)}
      names={names}
      onParam={(key, value) => update({ [key]: value })}
      onExport={target ? () => void commands.exportJson(target.routeId, target.selection, model.format) : null}
      onBulk={onBulk}
      filtersOnly={!onBulk}
    />
  );
}

/** Events: list / detail split with retry, discard, export, background jobs and edit-then-replay in the detail. */
function EventsView(props: ViewProps) {
  const { view, names, fixedRouteId, model, update } = props;
  const summary = useDeadLetterSummary();
  const [bulk, setBulk] = useState<DeadLetterJobAction | null>(null);
  const toolbar = useToolbar(props, setBulk);
  const { state } = model;
  const unresolved = DeadLetterViewCounts.unresolved(summary, view);
  const routeId = view.routeId;
  const startJob = (action: DeadLetterJobAction) => {
    setBulk(null);
    if (routeId) void model.startJob(routeId, action);
  };
  const retryNow = () => {
    setBulk(null);
    if (routeId) void model.retryAll(routeId);
  };
  const clear = () => update(DeadLetterViewParams.cleared(fixedRouteId));
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
