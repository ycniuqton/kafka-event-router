import { Button, Chip, Select } from '../../../components/ui';
import { Can } from '../../../access';
import {
  DeadLetterFileFormat,
  DeadLetterFileFormatLabel,
  DeadLetterJobAction,
  DeadLetterReports,
  DeadLetterTimeRange,
  DeadLetterTimeRanges,
  Permission,
  type DeadLetterKindCount,
} from '../../../domain';
import { DeadLetterCheckedRows, type DeadLetterRouteNames } from '../model/DeadLetterBrowserModel';
import { AllRoutesFilter, DeadLetterQuery, DeadLetterViewParams, type DeadLetterView } from '../model/DeadLetterQuery';
import type { DeadLetterBrowserModel } from '../model/useDeadLetterBrowser';
import { DeadLetterRefineBar } from './DeadLetterRefineBar';
import styles from './DeadLetterToolbar.module.css';

export type DeadLetterToolbarProps = {
  model: DeadLetterBrowserModel;
  view: DeadLetterView;
  fixedRouteId?: string;
  unresolved: number;
  approximate: boolean;
  kinds: DeadLetterKindCount[];
  names: DeadLetterRouteNames;
  onParam: (key: string, value: string | null) => void;
  onExport: (() => void) | null;
  onBulk: (action: DeadLetterJobAction) => void;
};

/** Count, route / error-type / time filters, the P2 refine bar and the bulk actions of the dead-letter list. */
export function DeadLetterToolbar(props: DeadLetterToolbarProps) {
  const { model, unresolved, approximate, view, onParam } = props;
  const { state } = model;
  const shown = `${state.items.length}${state.hasMore ? '+' : ''} shown`;
  return (
    <div className={styles.wrap}>
      <div className={styles.bar}>
        <input
          type="checkbox"
          className={styles.check}
          checked={state.allChecked}
          disabled={!state.items.length}
          aria-label="Select all"
          onChange={model.toggleAll}
        />
        <span className={styles.count}>
          {shown} · {approximate ? '~' : ''}
          {unresolved} unresolved
        </span>
        <DeadLetterFilters {...props} />
        <Button size="sm" variant="ghost" onClick={model.refresh} disabled={state.loading}>
          Refresh
        </Button>
        <DeadLetterBulkActions {...props} />
      </div>
      <DeadLetterRefineBar view={view} items={state.items} onParam={onParam} />
    </div>
  );
}

/** Route select (global screen only), time window and error-type chips. */
function DeadLetterFilters({ view, fixedRouteId, kinds, names, onParam }: DeadLetterToolbarProps) {
  return (
    <>
      {!fixedRouteId && (
        <Select controlSize="sm" value={view.routeId ?? AllRoutesFilter} aria-label="Route" onChange={(e) => onParam(DeadLetterQuery.Route, e.target.value || null)}>
          <option value={AllRoutesFilter}>All routes</option>
          {names.options(view.routeId).map((r) => (
            <option key={r.id} value={r.id}>
              {r.name}
            </option>
          ))}
        </Select>
      )}
      <Select controlSize="sm" value={view.range} aria-label="Time range" onChange={(e) => onParam(DeadLetterQuery.Range, e.target.value === DeadLetterTimeRange.All ? null : e.target.value)}>
        {Object.values(DeadLetterTimeRange).map((r) => (
          <option key={r} value={r}>
            {DeadLetterTimeRanges.labels[r]}
          </option>
        ))}
      </Select>
      {kinds.map(({ kind, count }) => (
        <Chip key={kind} active={view.kinds.includes(kind)} count={count} onClick={() => onParam(DeadLetterQuery.Kind, DeadLetterViewParams.toggleKind(view, kind))}>
          {kind}
        </Chip>
      ))}
    </>
  );
}

/** Retry / Discard / Restore the checked rows, or act on the whole view (large views run as background jobs); Export. */
function DeadLetterBulkActions(props: DeadLetterToolbarProps) {
  const { model, onExport } = props;
  const rows = model.state.checkedRows;
  const open = DeadLetterCheckedRows.unresolved(rows);
  const discarded = DeadLetterCheckedRows.discarded(rows);
  const formatLabel = DeadLetterFileFormatLabel[model.format];
  return (
    <Can permission={Permission.DlqAct}>
      {rows.length > 0 ? (
        <>
          {open.length > 0 && <Button size="sm" onClick={() => void model.retry(open)}>Retry {open.length}</Button>}
          {open.length > 0 && <Button size="sm" onClick={() => void model.discard(open)}>Discard {open.length}</Button>}
          {discarded.length > 0 && <Button size="sm" onClick={() => void model.restore(discarded)}>Restore {discarded.length}</Button>}
        </>
      ) : (
        <ViewActions {...props} />
      )}
      <Select controlSize="sm" value={model.format} aria-label="Export format" onChange={(e) => model.setFormat(e.target.value as DeadLetterFileFormat)}>
        {Object.values(DeadLetterFileFormat).map((f) => (
          <option key={f} value={f}>
            {DeadLetterFileFormatLabel[f]}
          </option>
        ))}
      </Select>
      <Button size="sm" disabled={!onExport} title={onExport ? undefined : 'Pick one route (or check rows of one route) to export'} onClick={() => onExport?.()}>
        Export {formatLabel}
      </Button>
    </Can>
  );
}

/** "Retry all" (sync up to 500, else a background job) and "Discard all" (background job) of the viewed route. */
function ViewActions({ model, view, unresolved, onBulk }: DeadLetterToolbarProps) {
  const { state } = model;
  const routeId = view.routeId;
  const large = unresolved > DeadLetterReports.syncCap;
  const retryAll = () => {
    if (!routeId) return void model.retry(DeadLetterCheckedRows.unresolved(state.items));
    if (large) return onBulk(DeadLetterJobAction.Retry);
    return void model.retryAll(routeId);
  };
  return (
    <>
      <Button size="sm" disabled={!state.items.length} title={routeId ? 'Retries every matching event (over 500 runs in the background)' : 'Retries the events shown'} onClick={retryAll}>
        Retry all
      </Button>
      {routeId && (
        <Button size="sm" disabled={!state.items.length} title="Discards every matching event in the background" onClick={() => onBulk(DeadLetterJobAction.Discard)}>
          Discard all…
        </Button>
      )}
    </>
  );
}
