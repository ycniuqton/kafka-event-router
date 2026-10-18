import { Card, CardHeader, Field, TextInput } from '../../../../components/ui';
import { RouteSettingsRules, type RouteLimits } from '../../../../domain';
import type { SettingsSectionProps } from './SettingsSectionProps';
import styles from './SettingsTab.module.css';

const limitKeys = Object.keys(RouteSettingsRules.limitLabels) as (keyof RouteLimits)[];

/** Processing limits; empty fields keep the engine defaults (saved into the draft). */
export function LimitsSection({ values, errors, onChange, readOnly }: SettingsSectionProps) {
  return (
    <Card padding="md" className={styles.section}>
      <CardHeader title="Limits" meta="Saved to the draft · applies on next deploy" />
      <div className={styles.grid}>
        {limitKeys.map((key) => (
          <Field key={key} label={RouteSettingsRules.limitLabels[key]} hint="Empty: default" error={errors[key]}>
            <TextInput
              block
              mono
              type="number"
              min={1}
              value={values.limits[key]}
              disabled={readOnly}
              onChange={(e) => onChange({ limits: { ...values.limits, [key]: e.target.value } })}
            />
          </Field>
        ))}
      </div>
    </Card>
  );
}
