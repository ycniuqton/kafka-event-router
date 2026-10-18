import type { CSSProperties } from 'react';
import { EventDocuments, MaskedValues, ProjectedState, TableColumns, type FieldSchema, type SampleRecord } from '../../domain';
import { MaskedMark } from './MaskedMark';
import { EventStatusBadge } from './EventStatusBadge';
import type { PanelRow } from './SamplePanelModel';
import styles from './TableView.module.css';

export type TableViewProps = { rows: PanelRow[]; schema: FieldSchema | null; records: SampleRecord[]; masked?: boolean };

/** Table view: one row per event, one column per frequent value field (from the inferred schema). */
export function TableView({ rows, schema, records, masked }: TableViewProps) {
  const columns = TableColumns.fromSchema(schema, records);
  const grid = { '--cols': columns.length } as CSSProperties;
  return (
    <div className={styles.table} style={grid}>
      <div className={`${styles.row} ${styles.head}`}>
        <span />
        <span>#</span>
        {columns.map((c) => (
          <span key={c.path} className={styles.cell} title={c.path}>
            {c.label}
          </span>
        ))}
      </div>
      {rows.map((row) => {
        const doc = EventDocuments.visible(row.projected.document);
        return (
          <div key={row.record.seq} className={styles.row} data-dim={row.projected.state === ProjectedState.Filtered || undefined}>
            <EventStatusBadge state={row.projected.state} />
            <span className={styles.n}>#{row.position + 1}</span>
            {columns.map((c) => (
              <TableCell key={c.path} text={TableColumns.cell(doc, c)} masked={masked} />
            ))}
          </div>
        );
      })}
    </div>
  );
}

/** One cell; masked values get the lock. */
function TableCell({ text, masked }: { text: string; masked?: boolean }) {
  return (
    <span className={styles.cell}>
      {text}
      {masked && MaskedValues.isMasked(text) && <MaskedMark />}
    </span>
  );
}
