import { Field, TextInput } from '../../../components/ui';
import { ConnectionField, ConnectionFieldErrors } from '../model/ConnectionFieldErrors';
import type { ConnectionFormStepProps } from '../model/ConnectionFormStepProps';
import { SecretInput } from './SecretInput';
import { TlsFields } from './TlsFields';
import { ToggleField } from './ToggleField';

/** Username + write-only password for SASL/PLAIN and SASL/SCRAM, optionally over TLS. */
export function SaslCredentials(props: ConnectionFormStepProps) {
  const { form, set, errors } = props;
  return (
    <>
      <Field label="Username" error={ConnectionFieldErrors.of(errors, ConnectionField.Username)}>
        <TextInput controlSize="lg" sunken value={form.username} placeholder="Username" onChange={(e) => set('username', e.target.value)} />
      </Field>
      <SecretInput
        value={form.password}
        placeholder="Password — write-only after saving"
        error={ConnectionFieldErrors.of(errors, ConnectionField.Password)}
        onChange={(v) => set('password', v)}
      />
      <ToggleField label="Over TLS (SASL_SSL)" checked={form.tlsEnabled} onChange={(v) => set('tlsEnabled', v)} />
      {form.tlsEnabled && <TlsFields {...props} />}
    </>
  );
}
