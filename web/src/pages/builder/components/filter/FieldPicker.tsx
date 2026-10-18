import { ControlSize, Select } from '../../../../components/ui';
import type { FilterFieldCatalog } from '../../model/FilterFieldCatalog';
import styles from './FilterRowEditor.module.css';

/** Longest example shown next to a field in the picker. */
const ExampleLength = { max: 24 } as const;

export type FieldPickerProps = {
  field: string;
  catalog: FilterFieldCatalog;
  onPick: (path: string) => void;
};

/** Field dropdown from the sample schema: path, type and an example value. */
export function FieldPicker({ field, catalog, onPick }: FieldPickerProps) {
  return (
    <Select controlSize={ControlSize.Md} mono sunken className={styles.field} value={field} onChange={(e) => onPick(e.target.value)} aria-label="Field">
      {catalog.conditionPaths(field).map((p) => {
        const example = catalog.example(p).slice(0, ExampleLength.max);
        return (
          <option key={p} value={p}>
            {p} · {catalog.typeOf(p)}
            {example ? ` · e.g. ${example}` : ''}
          </option>
        );
      })}
    </Select>
  );
}
