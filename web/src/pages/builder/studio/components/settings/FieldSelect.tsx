import { ControlSize, Select } from '../../../../../components/ui';
import { useStudio } from '../../hooks/StudioContext';
import { FieldOptions } from '../../model/FieldOptions';

type FieldSelectProps = { value: string; onChange(value: string): void; className?: string; ariaLabel?: string };

/** Select of the current sample's field paths (keeps a custom path that is not in the sample). */
export function FieldSelect({ value, onChange, className, ariaLabel = 'Field' }: FieldSelectProps) {
  const { model } = useStudio();
  return (
    <Select controlSize={ControlSize.Sm} mono sunken className={className} value={value} onChange={(e) => onChange(e.target.value)} aria-label={ariaLabel}>
      {FieldOptions.of(value, model.inputLines).map((path) => (
        <option key={path} value={path}>
          {path}
        </option>
      ))}
    </Select>
  );
}
