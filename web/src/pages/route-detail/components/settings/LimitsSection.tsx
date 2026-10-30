import { Card, CardHeader, Field, SegmentedControl, TextInput } from '../../../../components/ui';
import { InFlightMode, RouteSettingsRules, RouteTuningText, type RouteLimits, type RouteTuning } from '../../../../domain';
import type { SettingsSectionProps } from './SettingsSectionProps';
import styles from './SettingsTab.module.css';

const limitKeys = Object.keys(RouteSettingsRules.limitLabels) as (keyof RouteLimits)[];

const inFlightModeOptions = Object.values(InFlightMode).map((value) => ({ value, label: RouteSettingsRules.inFlightModeLabels[value] }));

type LimitsSectionProps = SettingsSectionProps & { tuning?: RouteTuning | null };

/** Processing limits; empty fields keep the engine defaults (saved into the draft). Shows the values the
 * runtime tuned itself to right now (in-flight limit, JS pool instances) next to the bounds being edited. */
export function LimitsSection({ values, errors, onChange, readOnly, tuning }: LimitsSectionProps) {
  const now = RouteTuningText.summary(tuning);
  return (
    <Card padding="md" className={styles.section}>
      <CardHeader title="Limits" meta="Saved to the draft · applies on next deploy" />
      <SegmentedControl
        options={inFlightModeOptions}
        value={values.inFlightMode}
        onChange={(inFlightMode) => !readOnly && onChange({ inFlightMode })}
        ariaLabel="In-flight sizing"
      />
      {now && <p className={styles.hint} data-testid="limits-auto-values">{now}</p>}
      <div className={styles.grid}>
        {limitKeys.map((key) => (
          <Field key={key} label={RouteSettingsRules.limitLabels[key]} hint={RouteSettingsRules.limitHint(key, values.inFlightMode)} error={errors[key]}>
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
