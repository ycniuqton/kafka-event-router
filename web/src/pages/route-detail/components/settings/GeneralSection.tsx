import { Card, CardHeader, Field, TextArea, TextInput } from '../../../../components/ui';
import { RouteSettingsRules } from '../../../../domain';
import { SettingsField } from '../../model/SettingsForm';
import type { SettingsSectionProps } from './SettingsSectionProps';
import styles from './SettingsTab.module.css';

/** Name, description, tags and replicas: saved on the route immediately. */
export function GeneralSection({ values, errors, onChange, readOnly }: SettingsSectionProps) {
  return (
    <Card padding="md" className={styles.section}>
      <CardHeader title="General" meta="Applies immediately" />
      <div className={styles.grid}>
        <Field label="Name" error={errors[SettingsField.Name]}>
          <TextInput block value={values.name} disabled={readOnly} onChange={(e) => onChange({ name: e.target.value })} />
        </Field>
        <Field label="Replicas" hint="Parallel runners of this route" error={errors[SettingsField.Replicas]}>
          <TextInput
            block
            mono
            type="number"
            min={RouteSettingsRules.minReplicas}
            max={RouteSettingsRules.maxReplicas}
            value={values.replicas}
            disabled={readOnly}
            onChange={(e) => onChange({ replicas: e.target.value })}
          />
        </Field>
      </div>
      <Field label="Description">
        <TextArea rows={2} value={values.description} disabled={readOnly} onChange={(e) => onChange({ description: e.target.value })} />
      </Field>
      <Field label="Tags" hint="Comma separated, e.g. pii, payments" error={errors[SettingsField.Tags]}>
        <TextInput block mono value={values.tags} disabled={readOnly} onChange={(e) => onChange({ tags: e.target.value })} />
      </Field>
    </Card>
  );
}
