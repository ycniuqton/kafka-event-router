import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, type CSSProperties } from 'react';
import { Button, DataTable, EmptyState, Format, TableCell, TableHead, TableRow } from '../../../components/ui';
import { AuditActions, type AuditEntry } from '../../../domain';
import { AuditLayout } from '../model/AuditFilter';
import { VirtualWindow } from '../model/VirtualWindow';
import styles from './AuditTable.module.css';

export type AuditTableProps = {
  entries: AuditEntry[];
  loading: boolean;
  error?: Error;
  hasMore: boolean;
  filtered: boolean;
  onLoadMore: () => void;
  onReset: () => void;
  onSelect: (entry: AuditEntry) => void;
};

const windowing = new VirtualWindow(AuditLayout.rowHeight, AuditLayout.overscan);

/** Read-only, virtualised rows (time, who, what, object, IP); sensitive rows amber; scrolling near the end loads the next page. */
export function AuditTable(props: AuditTableProps) {
  const { entries, loading, error, filtered, onReset } = props;
  if (error && !entries.length) return <EmptyState title="The audit log could not be loaded." description={error.message} />;
  if (!loading && !entries.length) {
    return (
      <EmptyState
        title={filtered ? 'No actions match these filters.' : 'No actions recorded yet.'}
        action={filtered ? <Button onClick={onReset}>Clear filters</Button> : undefined}
      />
    );
  }
  return (
    <DataTable columns={AuditLayout.columns} minWidth={AuditLayout.minWidth}>
      <TableHead>
        <span>Time</span>
        <span>Who</span>
        <span>Action</span>
        <span>Object</span>
        <span>IP</span>
      </TableHead>
      <AuditRows {...props} />
    </DataTable>
  );
}

function AuditRows({ entries, loading, hasMore, onLoadMore, onSelect }: AuditTableProps) {
  const viewport = useRef<HTMLDivElement>(null);
  const [scroll, setScroll] = useState({ top: 0, height: AuditLayout.rowHeight * AuditLayout.viewportRows });
  const measure = useCallback(() => {
    const el = viewport.current;
    if (el) setScroll({ top: el.scrollTop, height: el.clientHeight });
  }, []);
  useLayoutEffect(() => measure(), [measure]);
  const slice = useMemo(() => windowing.slice(entries.length, scroll.top, scroll.height), [entries.length, scroll]);
  const nearEnd = windowing.nearEnd(entries.length, scroll.top, scroll.height, AuditLayout.loadAheadRows);
  useEffect(() => {
    if (nearEnd && hasMore && !loading) onLoadMore();
  }, [nearEnd, hasMore, loading, onLoadMore]);
  const vars = { '--viewport-h': `${AuditLayout.rowHeight * AuditLayout.viewportRows}px`, '--row-h': `${AuditLayout.rowHeight}px` } as CSSProperties;
  return (
    <div ref={viewport} className={styles.viewport} style={vars} onScroll={measure} data-testid="audit-rows">
      <div className={styles.spacer} style={{ '--pad': `${slice.padTop}px` } as CSSProperties} />
      {entries.slice(slice.start, slice.end).map((e) => <AuditRow key={e.id} entry={e} onSelect={onSelect} />)}
      <div className={styles.spacer} style={{ '--pad': `${slice.padBottom}px` } as CSSProperties} />
      {loading && <div className={styles.status}>Loading…</div>}
    </div>
  );
}

function AuditRow({ entry: e, onSelect }: { entry: AuditEntry; onSelect: (entry: AuditEntry) => void }) {
  return (
    <TableRow className={[styles.row, e.sensitive && styles.sensitive].filter(Boolean).join(' ')} onClick={() => onSelect(e)}
      data-sensitive={e.sensitive || undefined}>
      <TableCell className={styles.mono3}>{Format.monthDayTime(e.at)}</TableCell>
      <TableCell>{e.actor}</TableCell>
      <TableCell className={styles.action} title={e.action}>{AuditActions.label(e.action)}</TableCell>
      <TableCell className={styles.mono2} title={e.objectId}>{e.object}</TableCell>
      <TableCell className={styles.mono3}>{e.ip}</TableCell>
    </TableRow>
  );
}
