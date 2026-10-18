import type { FilterRow, RowResult } from '../../../../domain';
import type { FilterFieldCatalog } from '../../model/FilterFieldCatalog';
import { FieldPicker } from './FieldPicker';
import { OperatorSelect } from './OperatorSelect';
import { ValueInput } from './ValueInput';
import styles from './FilterRowEditor.module.css';

export type FilterRowEditorProps = {
  row: FilterRow;
  catalog: FilterFieldCatalog;
  /** Result of this row for the "Why?" sample (true/false), or null when not evaluated / not explained. */
  result: RowResult | undefined;
  /** Highlights a row just added from the sample panel. */
  fresh?: boolean;
  onReplace: (row: FilterRow, change: string) => void;
  onDelete: () => void;
};

/** One condition row (ConditionRow): `[field ▾] [operator ▾] [value]`, why-filtered tone and delete. */
export function FilterRowEditor({ row, catalog, result, fresh, onReplace, onDelete }: FilterRowEditorProps) {
  const explained = result === true || result === false;
  return (
    <div className={styles.row} data-result={explained ? String(result) : undefined} data-fresh={fresh || undefined}>
      <FieldPicker field={row.field} catalog={catalog} onPick={(path) => onReplace(catalog.rowFor(path), 'Filter condition changed')} />
      <OperatorSelect op={row.op} ops={catalog.opsFor(row.field)} onChange={(op) => onReplace({ ...row, op }, 'Filter operator changed')} />
      <ValueInput row={row} catalog={catalog} onChange={(next) => onReplace(next, 'Filter value changed')} />
      <span className={styles.result}>{explained && (result ? '✓ true' : '✕ false')}</span>
      <button type="button" className={styles.del} onClick={onDelete} aria-label="Remove condition">
        ×
      </button>
    </div>
  );
}
