import { Select } from '../../../components/ui';
import { ScramMechanism } from '../model/ConnectionForm';
import type { ConnectionFormStepProps } from '../model/ConnectionFormStepProps';
import { SaslCredentials } from './SaslCredentials';

/** SASL/SCRAM: hash mechanism (512 / 256) plus credentials. */
export function ScramFields(props: ConnectionFormStepProps) {
  const { form, set } = props;
  return (
    <>
      <Select controlSize="lg" sunken value={form.scram} aria-label="SCRAM mechanism" onChange={(e) => set('scram', e.target.value as ScramMechanism)}>
        {Object.values(ScramMechanism).map((m) => (
          <option key={m}>{m}</option>
        ))}
      </Select>
      <SaslCredentials {...props} />
    </>
  );
}
