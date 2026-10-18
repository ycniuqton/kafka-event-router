import { Field, SegmentedControl, TextInput } from '../../../components/ui';
import { RegistryAuthMode, RegistryAuthRules } from '../../../domain';
import { ConnectionField, ConnectionFieldErrors } from '../model/ConnectionFieldErrors';
import type { ConnectionFormStepProps } from '../model/ConnectionFormStepProps';
import { AdvancedProperties } from './AdvancedProperties';
import { SecretInput } from './SecretInput';
import styles from './SchemaRegistryStep.module.css';

/** Texts of the schema registry step. */
const RegistryText = {
  hint: 'Optional. When set, the test checks that the registry answers, and Avro / Protobuf / JSON Schema events can be decoded.',
  url: 'Schema registry URL',
  urlHint: 'Leave empty for clusters without a registry.',
  auth: 'Authentication',
  username: 'Username',
  password: 'Password — write-only after saving',
  token: 'Bearer token — write-only after saving',
} as const;

/** Secret placeholder per auth mode (only modes that use a secret show the input). */
const SecretPlaceholder: Record<RegistryAuthMode, string> = {
  [RegistryAuthMode.None]: '',
  [RegistryAuthMode.Basic]: RegistryText.password,
  [RegistryAuthMode.Bearer]: RegistryText.token,
};

const AuthOptions = Object.values(RegistryAuthMode).map((mode) => ({ value: mode, label: RegistryAuthRules.labels[mode] }));

/** Step 4 — optional schema registry (URL, auth mode, username, write-only secret) plus the Advanced raw properties. */
export function SchemaRegistryStep({ form, set, errors }: ConnectionFormStepProps) {
  const error = (field: string) => ConnectionFieldErrors.of(errors, field);
  const configured = form.registryUrl.trim() !== '';
  return (
    <>
      <span className={styles.hint}>{RegistryText.hint}</span>
      <Field label={RegistryText.url} hint={RegistryText.urlHint} error={error(ConnectionField.RegistryUrl)}>
        <TextInput controlSize="lg" mono sunken value={form.registryUrl} placeholder="https://schema-registry.internal:8081" onChange={(e) => set('registryUrl', e.target.value)} />
      </Field>
      {configured && (
        <div className={styles.group}>
          <span className={styles.label}>{RegistryText.auth}</span>
          <SegmentedControl size="md" options={AuthOptions} value={form.registryAuth} onChange={(mode) => set('registryAuth', mode)} ariaLabel={RegistryText.auth} />
          {error(ConnectionField.RegistryAuth) && <span className={styles.error}>{error(ConnectionField.RegistryAuth)}</span>}
        </div>
      )}
      {configured && RegistryAuthRules.usesUsername(form.registryAuth) && (
        <Field label={RegistryText.username} error={error(ConnectionField.RegistryUser)}>
          <TextInput controlSize="lg" sunken value={form.registryUser} onChange={(e) => set('registryUser', e.target.value)} />
        </Field>
      )}
      {configured && RegistryAuthRules.usesSecret(form.registryAuth) && (
        <SecretInput
          value={form.registryPassword}
          placeholder={SecretPlaceholder[form.registryAuth]}
          error={error(ConnectionField.RegistrySecret)}
          onChange={(v) => set('registryPassword', v)}
        />
      )}
      <AdvancedProperties rows={form.properties} errors={errors} onChange={(rows) => set('properties', rows)} />
    </>
  );
}
