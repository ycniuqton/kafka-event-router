import { ControlSize, Select, TextInput } from '../../../../components/ui';
import { FilterLiteral, FilterOp, type FilterRow } from '../../../../domain';
import type { FilterFieldCatalog } from '../../model/FilterFieldCatalog';
import styles from './FilterRowEditor.module.css';

const booleanValues = [FilterLiteral.True, FilterLiteral.False];

export type ValueInputProps = {
  row: FilterRow;
  catalog: FilterFieldCatalog;
  onChange: (row: FilterRow) => void;
};

type ValueOption = { value: string; label: string };

/** Adaptive value: nothing for "exists", true/false for booleans, seen values with frequency, else free text. */
export function ValueInput({ row, catalog, onChange }: ValueInputProps) {
  if (row.op === FilterOp.Exists) return <span className={styles.value} />;
  if (row.literal || catalog.isBoolean(row.field)) {
    const values = (booleanValues as string[]).includes(row.value) ? booleanValues : [row.value, ...booleanValues];
    return <ValueSelect value={row.value} options={values.map((v) => ({ value: v, label: v }))} onChange={(value) => onChange({ ...row, value, literal: true })} />;
  }
  const options = catalog.valueOptions(row);
  if (options) return <ValueSelect value={row.value} options={options} onChange={(value) => onChange({ ...row, value })} />;
  return <TextInput controlSize={ControlSize.Md} mono sunken className={styles.value} value={row.value} onChange={(e) => onChange({ ...row, value: e.target.value })} aria-label="Value" />;
}

/** Value dropdown (booleans or seen values with frequency). */
function ValueSelect({ value, options, onChange }: { value: string; options: ValueOption[]; onChange: (value: string) => void }) {
  return (
    <Select controlSize={ControlSize.Md} mono sunken className={styles.value} value={value} onChange={(e) => onChange(e.target.value)} aria-label="Value">
      {options.map((o) => (
        <option key={o.value} value={o.value}>
          {o.label}
        </option>
      ))}
    </Select>
  );
}
