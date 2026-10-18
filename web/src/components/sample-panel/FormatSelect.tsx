import { ValueFormat } from '../../domain';
import { ControlSize, Select } from '../ui';
import styles from './SampleToolbar.module.css';

export type FormatSelectProps = { value: ValueFormat; disabled: boolean; onChange: (format: ValueFormat) => void };

/** Labels of the decode formats; avro / protobuf decode through the connection's schema registry. */
const FormatLabels: Record<ValueFormat, string> = {
  [ValueFormat.Json]: 'JSON',
  [ValueFormat.String]: 'String',
  [ValueFormat.Bytes]: 'Bytes',
  [ValueFormat.Avro]: 'Avro (registry)',
  [ValueFormat.Protobuf]: 'Protobuf (registry)',
};

/** Per-session format override (FR-8): re-decodes the loaded raw bytes without re-reading Kafka. */
export function FormatSelect({ value, disabled, onChange }: FormatSelectProps) {
  return (
    <label className={styles.format}>
      <span className={styles.formatLabel}>Decode as</span>
      <Select controlSize={ControlSize.Sm} value={value} disabled={disabled} aria-label="Decode samples as" data-testid="sample-format"
        onChange={(e) => onChange(e.target.value as ValueFormat)}>
        {Object.values(ValueFormat).map((f) => (
          <option key={f} value={f}>
            {FormatLabels[f]}
          </option>
        ))}
      </Select>
    </label>
  );
}
