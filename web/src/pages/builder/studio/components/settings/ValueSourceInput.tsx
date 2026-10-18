import { ControlSize, SegmentedControl, SegmentSize, Select, TextInput } from '../../../../../components/ui';
import { MetadataName, ValueSourceKind, ValueSourceLabel, type TransformStep } from '../../../../../domain';
import { FormulaBar } from '../../mapper/FormulaBar';
import { useStudio } from '../../hooks/StudioContext';
import { FieldSelect } from './FieldSelect';
import styles from './RecipeSettings.module.css';

type ValueSourceInputProps = { step: TransformStep; fallback: ValueSourceKind; onChange(patch: Partial<TransformStep>): void };

/** Value source of set / default / key: Fixed value (JSON literals keep their type) · From field · From metadata. */
export function ValueSourceInput({ step, fallback, onChange }: ValueSourceInputProps) {
  const kind = step.sourceKind ?? fallback;
  const { model } = useStudio();
  const kinds = Object.values(ValueSourceKind).map((k) => ({ value: k, label: ValueSourceLabel[k] }));
  return (
    <>
      <div className={styles.start}>
        <SegmentedControl size={SegmentSize.Xs} options={kinds} value={kind} onChange={(sourceKind) => onChange({ sourceKind, argument: '' })} ariaLabel="Value source" />
      </div>
      {kind === ValueSourceKind.Constant && (
        <TextInput controlSize={ControlSize.Sm} mono sunken placeholder='value, 42, true or "text"' value={step.argument} onChange={(e) => onChange({ argument: e.target.value })} aria-label="Fixed value" />
      )}
      {kind === ValueSourceKind.Formula && (
        <FormulaBar value={step.argument} onChange={(argument) => onChange({ argument })} fields={model.inputLines.filter((l) => !l.isBranch).map((l) => l.path)} />
      )}
      {kind === ValueSourceKind.Field && <FieldSelect value={step.argument} onChange={(argument) => onChange({ argument })} />}
      {kind === ValueSourceKind.Metadata && (
        <Select controlSize={ControlSize.Sm} mono sunken value={step.argument} onChange={(e) => onChange({ argument: e.target.value })} aria-label="Metadata">
          <option value="">choose…</option>
          {Object.values(MetadataName).map((m) => (
            <option key={m} value={m}>
              {m}
            </option>
          ))}
        </Select>
      )}
    </>
  );
}
