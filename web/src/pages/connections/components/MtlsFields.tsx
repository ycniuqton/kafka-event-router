import { ConnectionField, ConnectionFieldErrors } from '../model/ConnectionFieldErrors';
import { ConnectionFormModel } from '../model/ConnectionForm';
import type { ConnectionFormStepProps } from '../model/ConnectionFormStepProps';
import { PemDropZone } from './PemDropZone';
import { TlsFields } from './TlsFields';

/** mTLS: CA, client certificate and the write-only client key as dropped .pem files. */
export function MtlsFields(props: ConnectionFormStepProps) {
  const { form, set, errors } = props;
  return (
    <>
      <TlsFields {...props} />
      <PemDropZone
        label="Client certificate"
        value={form.clientCertPem}
        error={ConnectionFieldErrors.of(errors, ConnectionField.ClientCertPem)}
        onChange={(v) => set('clientCertPem', v)}
      />
      <PemDropZone
        label="Client private key — write-only after saving"
        value={form.clientKey.value}
        stored={form.clientKey.stored}
        error={ConnectionFieldErrors.of(errors, ConnectionField.ClientKey)}
        onChange={(v) => set('clientKey', v ? { value: v, stored: false } : ConnectionFormModel.emptySecret)}
      />
    </>
  );
}
