import { Field, TextInput } from '../../../../components/ui';
import { RetryPolicyRules } from '../../../../domain';
import { RetryForm, SettingsField, type RetryFormValues } from '../../model/SettingsForm';
import type { SettingsSectionProps } from './SettingsSectionProps';
import styles from './SettingsTab.module.css';

/** Auto-retry of transient failures (timeouts, lookups or registries unavailable) through delayed retry topics. */
export function RetryPolicyFields({ values, errors, onChange, readOnly }: SettingsSectionProps) {
  const retry = values.retry;
  const set = (patch: Partial<RetryFormValues>) => onChange({ retry: { ...retry, ...patch } });
  const policy = RetryForm.policy(retry);
  const valid = Object.keys(RetryPolicyRules.validate(policy)).length === 0;
  return (
    <div className={styles.retry}>
      <label className={styles.toggle}>
        <input type="checkbox" checked={retry.enabled} disabled={readOnly} onChange={(e) => set({ enabled: e.target.checked })} />
        Retry transient failures later without blocking the partition
      </label>
      {retry.enabled && (
        <>
          <div className={styles.grid}>
            <Field label="Attempts" hint={`${RetryPolicyRules.attempts.min}–${RetryPolicyRules.attempts.max}`} error={errors[SettingsField.RetryAttempts]}>
              <TextInput block mono type="number" min={RetryPolicyRules.attempts.min} max={RetryPolicyRules.attempts.max} value={retry.maxAttempts} disabled={readOnly} onChange={(e) => set({ maxAttempts: e.target.value })} />
            </Field>
            <Field label="Base delay (ms)" hint="Delay of the first attempt" error={errors[SettingsField.RetryBase]}>
              <TextInput block mono type="number" min={RetryPolicyRules.baseDelay.min} value={retry.baseDelayMs} disabled={readOnly} onChange={(e) => set({ baseDelayMs: e.target.value })} />
            </Field>
            <Field label="Max delay (ms)" hint="Cap of one attempt's delay" error={errors[SettingsField.RetryMax]}>
              <TextInput block mono type="number" min={RetryPolicyRules.maxDelay.min} value={retry.maxDelayMs} disabled={readOnly} onChange={(e) => set({ maxDelayMs: e.target.value })} />
            </Field>
          </div>
          <div className={styles.hint}>
            {valid ? `Schedule: ${RetryPolicyRules.schedule(policy)}, then the error policy applies (dead letters show the attempts).` : 'Fix the values to see the schedule.'}
          </div>
        </>
      )}
    </div>
  );
}
