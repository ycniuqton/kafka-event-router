import { Field, TextInput } from '../../../components/ui';
import { ConnectionField, ConnectionFieldErrors } from '../model/ConnectionFieldErrors';
import type { ConnectionFormStepProps } from '../model/ConnectionFormStepProps';
import { PemDropZone } from './PemDropZone';

/** Texts of the TLS fields. */
const TlsText = {
  ca: 'CA certificate (optional — system trust store when empty)',
  serverName: 'TLS server name (optional)',
  serverNameHint: 'Only when the broker certificate name differs from the bootstrap host.',
} as const;

/** TLS trust settings: CA certificate and server-name override. */
export function TlsFields({ form, set, errors }: ConnectionFormStepProps) {
  return (
    <>
      <PemDropZone label={TlsText.ca} value={form.caPem} error={ConnectionFieldErrors.of(errors, ConnectionField.CaPem)} onChange={(v) => set('caPem', v)} />
      <Field label={TlsText.serverName} hint={TlsText.serverNameHint} error={ConnectionFieldErrors.of(errors, ConnectionField.ServerName)}>
        <TextInput controlSize="lg" mono sunken value={form.serverName} placeholder="kafka.internal" onChange={(e) => set('serverName', e.target.value)} />
      </Field>
    </>
  );
}
