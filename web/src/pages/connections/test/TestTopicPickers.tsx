import { Field, Select } from '../../../components/ui';
import type { ConnectionTestOptions } from '../../../domain';
import styles from './TestChecklist.module.css';

/** Texts of the read / write topic pickers. */
const PickerText = {
  read: 'Read check topic',
  readAuto: 'Any topic (first non-internal)',
  write: 'Write check topic (optional)',
  writeSkip: 'Skip the write check',
  writeHint: 'Writes one test event with header x-router-test to an existing topic.',
} as const;

export type TestTopicPickersProps = {
  topics: string[];
  options: ConnectionTestOptions;
  disabled: boolean;
  onChange: (options: ConnectionTestOptions) => void;
};

/** Optional topics for the "can read" and "can write" checks (CN-07). */
export function TestTopicPickers({ topics, options, disabled, onChange }: TestTopicPickersProps) {
  const pick = (value: string) => value || undefined;
  return (
    <div className={styles.pickers}>
      <Field label={PickerText.read}>
        <Select controlSize="md" mono sunken disabled={disabled} value={options.readTopic ?? ''} onChange={(e) => onChange({ ...options, readTopic: pick(e.target.value) })}>
          <option value="">{PickerText.readAuto}</option>
          {topics.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </Select>
      </Field>
      <Field label={PickerText.write} hint={PickerText.writeHint}>
        <Select controlSize="md" mono sunken disabled={disabled} value={options.writeTopic ?? ''} onChange={(e) => onChange({ ...options, writeTopic: pick(e.target.value) })}>
          <option value="">{PickerText.writeSkip}</option>
          {topics.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </Select>
      </Field>
    </div>
  );
}
