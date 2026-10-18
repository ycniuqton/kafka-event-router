import { ControlSize, TextInput } from '../../../../components/ui';
import type { HeaderRow } from '../../../../domain';
import { DestinationExtrasRules } from '../../model/DestinationExtras';
import styles from './HeaderRowsEditor.module.css';

export type HeaderRowsEditorProps = { rows: HeaderRow[]; onChange: (rows: HeaderRow[], change: string) => void };

/** Key/value headers added to every output record. */
export function HeaderRowsEditor({ rows, onChange }: HeaderRowsEditorProps) {
  const update = (index: number, patch: Partial<HeaderRow>) => onChange(rows.map((r, i) => (i === index ? { ...r, ...patch } : r)), 'Headers edited');
  return (
    <div className={styles.rows}>
      {rows.map((row, i) => (
        <div key={i} className={styles.row}>
          <TextInput controlSize={ControlSize.Sm} mono placeholder="header" value={row.key} onChange={(e) => update(i, { key: e.target.value })} />
          <span className={styles.eq}>=</span>
          <TextInput controlSize={ControlSize.Sm} mono placeholder="value" value={row.value} onChange={(e) => update(i, { value: e.target.value })} />
          <button type="button" className={styles.del} aria-label="Remove header" onClick={() => onChange(rows.filter((_, j) => j !== i), 'Removed header')}>
            ×
          </button>
        </div>
      ))}
      <button type="button" className={styles.add} onClick={() => onChange([...rows, { ...DestinationExtrasRules.blankHeader }], 'Added header')}>
        + Add header
      </button>
    </div>
  );
}
