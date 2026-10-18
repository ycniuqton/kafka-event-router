import { MaskedHint } from '../../access/MaskedHint';
import { useMemo, useState } from 'react';
import { PanelValueActionKind, SampleView, ValueFormat, type JsonLine, type PanelValueAction, type ProjectionPoint } from '../../domain';
import { BranchSelector } from './BranchSelector';
import type { PreviewActions, PreviewState, SampleSourceRef } from '../../store';
import { EmptyState, SegmentedControl, SegmentSize, type LineDecoration } from '../ui';
import { EventPager } from './EventPager';
import { PanelMessages } from './PanelMessages';
import { PrettyView } from './PrettyView';
import { RawView } from './RawView';
import { SamplePanelModel, type PanelRow } from './SamplePanelModel';
import { SampleToolbar } from './SampleToolbar';
import { SummaryBar } from './SummaryBar';
import { TableView } from './TableView';
import { ValueActionLabels } from './ValueActionLabels';
import { ValueContextMenu, type ValueMenuTarget } from './ValueContextMenu';
import styles from './SamplePanel.module.css';

export type SamplePanelProps = {
  preview: { state: PreviewState; actions: PreviewActions };
  point: ProjectionPoint;
  /** Header suffix, e.g. "as read", "after Filter", "final output". */
  stageLabel: string;
  tip?: string;
  /** Source of latest / time-range loads; null allows uploads only. */
  source: SampleSourceRef | null;
  connectionName: string;
  valueActions?: readonly PanelValueActionKind[];
  onValueAction?: (action: PanelValueAction) => void;
  whySeq?: number | null;
  onWhy?: (seq: number) => void;
  /** Reason line of a filtered event at a position (e.g. the failed condition). */
  reasonFor?: (position: number) => string | undefined;
  defaultOpen?: boolean;
};

/** View options of the panel header. */
const ViewOptions = [
  { value: SampleView.Pretty, label: 'Pretty' },
  { value: SampleView.Table, label: 'Table' },
  { value: SampleView.Raw, label: 'Raw' },
];

/** Live sample panel (spec §9): load menu, summary, Pretty / Table / Raw views of events after the selected step. */
export function SamplePanel(props: SamplePanelProps) {
  const { state, actions } = props.preview;
  const [open, setOpen] = useState(props.defaultOpen ?? true);
  const [page, setPage] = useState(0);
  const [toolbarError, setToolbarError] = useState<string | null>(null);
  const { connectionName, source, reasonFor } = props;
  const point = useMemo(() => ({ ...props.point, branch: state.branch }), [props.point, state.branch]);
  const model = useMemo(
    () => new SamplePanelModel(state, point, { connectionName, topics: source?.topics ?? [] }, reasonFor),
    [state, point, connectionName, source, reasonFor],
  );
  const format = state.session ? (state.session.format ?? ValueFormat.Json) : null;
  if (!open) {
    return (
      <button type="button" className={styles.collapsed} onClick={() => setOpen(true)}>
        Live samples
      </button>
    );
  }
  return (
    <aside className={styles.panel} data-testid="sample-panel">
      <div className={styles.head}>
        <div className={styles.titleRow}>
          <span className={styles.title}>
            Live samples <span className={styles.stage}>· {props.stageLabel}</span>
          </span>
          <SegmentedControl options={ViewOptions} value={state.view} onChange={(v) => actions.setView(v)} size={SegmentSize.Xs} ariaLabel="Sample view" />
          <button type="button" className={styles.collapse} title="Collapse" onClick={() => setOpen(false)}>
            ⇥
          </button>
        </div>
        <SampleToolbar
          hasSource={!!source}
          loading={!!model.busy}
          query={state.query}
          onLatest={(count) => source && void actions.loadLatest(source, count)}
          onRange={(from, to) => source && void actions.loadTimeRange(source, from, to)}
          onUpload={(text) => void actions.upload(text, source ?? undefined)}
          onSearch={(q) => void actions.search(q)}
          onError={setToolbarError}
          format={format}
          onFormat={(f) => void actions.reformat(f)}
        />
        <BranchSelector branches={state.result?.summary.branches ?? []} value={state.branch} onChange={(b) => actions.setBranch(b)} />
        <SummaryBar summary={model.summary} notices={model.notices} busy={model.busy} error={toolbarError ?? model.runError} tip={props.tip} />
      </div>
      <div className={styles.body}>
        {(state.session?.masked || state.result?.masked) && <MaskedHint />}
        <PanelBody {...props} point={point} model={model} page={page} />
      </div>
      <EventPager page={Math.min(page, model.pageCount() - 1)} total={model.rows.length} onPage={setPage} />
    </aside>
  );
}

type PanelBodyProps = SamplePanelProps & { model: SamplePanelModel; page: number };

/** Empty / error state, or the selected view of the current page. */
function PanelBody({ preview, model, page, source, valueActions = [], onValueAction, whySeq, onWhy }: PanelBodyProps) {
  const [menu, setMenu] = useState<ValueMenuTarget | null>(null);
  const { state } = preview;
  if (model.loadError) return <EmptyState compact title="Couldn't load samples" description={model.loadError} />;
  if (!state.session && !source) return <EmptyState compact title="No samples" description={PanelMessages.noSource} />;
  if (!state.session) return <EmptyState compact title={model.busy ?? PanelMessages.loading} />;
  const rows = model.page(page);
  const masked = !!(state.session?.masked || state.result?.masked);
  const kinds = (row: PanelRow) => valueActions.filter((k) => k !== PanelValueActionKind.KeepOnly || !row.projected.changed);
  const onValue = (row: PanelRow, line: JsonLine, x: number, y: number) => {
    const allowed = kinds(row);
    if (allowed.length === 1) onValueAction?.({ kind: allowed[0], path: line.path, value: line.value });
    else if (allowed.length > 1) setMenu({ x, y, path: line.path, value: line.value });
  };
  const title = (line: JsonLine): LineDecoration | undefined =>
    line.isBranch || valueActions.length !== 1 ? undefined : { title: ValueActionLabels.labels[valueActions[0]](line.path, line.value) };
  return (
    <>
      {state.view === SampleView.Pretty && (
        <PrettyView rows={rows} whySeq={whySeq} onWhy={onWhy} onValue={onValueAction && valueActions.length ? onValue : undefined} valueTitle={title} masked={masked} />
      )}
      {state.view === SampleView.Table && <TableView rows={rows} schema={state.schema} records={state.records} masked={masked} />}
      {state.view === SampleView.Raw && <RawView rows={rows} masked={masked} />}
      <ValueContextMenu target={menu} kinds={valueActions} onAction={(a) => onValueAction?.(a)} onClose={() => setMenu(null)} />
    </>
  );
}
