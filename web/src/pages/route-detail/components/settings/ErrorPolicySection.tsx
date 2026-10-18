import { Card, CardHeader, Field, SegmentedControl, Select, TextInput } from '../../../../components/ui';
import { ErrorAction, RouteSettingsRules } from '../../../../domain';
import { useConnections } from '../../../../store';
import { SettingsField } from '../../model/SettingsForm';
import type { SettingsSectionProps } from './SettingsSectionProps';
import styles from './SettingsTab.module.css';

const errorActionOptions = Object.values(ErrorAction).map((value) => ({ value, label: RouteSettingsRules.errorActionLabels[value] }));

/** Value of the DLQ connection dropdown meaning "the source connection". */
const SourceConnection = '';

/** On-error behaviour with the dead-letter connection, topic and retries (saved into the draft). */
export function ErrorPolicySection({ values, errors, onChange, readOnly }: SettingsSectionProps) {
  const connections = useConnections();
  const dlq = values.onError === ErrorAction.DeadLetter;
  return (
    <Card padding="md" className={styles.section}>
      <CardHeader title="Error policy" meta="Saved to the draft · applies on next deploy" />
      <SegmentedControl options={errorActionOptions} value={values.onError} onChange={(onError) => !readOnly && onChange({ onError })} ariaLabel="On error" />
      <div className={styles.grid}>
        <Field label="Dead-letter connection" hint="Defaults to the source connection">
          <Select block value={values.dlqConnectionId} disabled={readOnly || !dlq} onChange={(e) => onChange({ dlqConnectionId: e.target.value })}>
            <option value={SourceConnection}>Same as source</option>
            {connections.map((c) => (
              <option key={c.id} value={c.id}>
                {c.name ?? c.id}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Dead-letter topic" hint="Empty: router.dlq.<route>">
          <TextInput block mono value={values.dlqTopic} disabled={readOnly || !dlq} onChange={(e) => onChange({ dlqTopic: e.target.value })} />
        </Field>
        <Field label="Max retries" hint="Empty: engine default" error={errors[SettingsField.Retries]}>
          <TextInput block mono type="number" min={1} value={values.maxRetries} disabled={readOnly} onChange={(e) => onChange({ maxRetries: e.target.value })} />
        </Field>
      </div>
    </Card>
  );
}
