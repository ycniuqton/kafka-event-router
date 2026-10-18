import { ControlSize, Select, TextInput } from '../../../../components/ui';
import { MetadataName, ValueSourceKind, ValueSourceLabel } from '../../../../domain';
import { FieldSelect } from '../components/settings/FieldSelect';
import { StudioText } from '../model/StudioText';
import { FormulaBar } from './FormulaBar';
import styles from './VisualMapper.module.css';

/** A value source as the mapper and set steps edit it. */
export type SourceValue = { sourceKind: ValueSourceKind; argument: string };

type ValueSourcePickerProps = SourceValue & {
  onChange(patch: Partial<SourceValue>): void;
  fields: string[];
  live?: unknown;
};

/** Value source of one output field: From field · Fixed value · Formula · From metadata. */
export function ValueSourcePicker({ sourceKind, argument, onChange, fields, live }: ValueSourcePickerProps) {
  return (
    <div className={styles.source}>
      <Select controlSize={ControlSize.Sm} sunken className={styles.kind} value={sourceKind} aria-label="Value source" onChange={(e) => onChange({ sourceKind: e.target.value as ValueSourceKind, argument: '' })}>
        {Object.values(ValueSourceKind).map((k) => (
          <option key={k} value={k}>
            {ValueSourceLabel[k]}
          </option>
        ))}
      </Select>
      <SourceEditor sourceKind={sourceKind} argument={argument} onChange={onChange} fields={fields} live={live} />
    </div>
  );
}

/** The editor of the chosen source kind. */
function SourceEditor({ sourceKind, argument, onChange, fields, live }: ValueSourcePickerProps) {
  if (sourceKind === ValueSourceKind.Field) return <FieldSelect className={styles.grow} value={argument} onChange={(a) => onChange({ argument: a })} />;
  if (sourceKind === ValueSourceKind.Formula) return <FormulaBar value={argument} onChange={(a) => onChange({ argument: a })} fields={fields} live={live} />;
  if (sourceKind === ValueSourceKind.Metadata) {
    return (
      <Select controlSize={ControlSize.Sm} mono sunken className={styles.grow} value={argument} aria-label="Metadata" onChange={(e) => onChange({ argument: e.target.value })}>
        <option value="">{StudioText.choose}</option>
        {Object.values(MetadataName).map((m) => (
          <option key={m} value={m}>
            {m}
          </option>
        ))}
      </Select>
    );
  }
  return <TextInput controlSize={ControlSize.Sm} mono sunken className={styles.grow} placeholder={StudioText.constantPlaceholder} value={argument} aria-label="Fixed value" onChange={(e) => onChange({ argument: e.target.value })} />;
}
