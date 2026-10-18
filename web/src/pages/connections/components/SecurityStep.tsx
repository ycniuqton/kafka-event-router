import type { ComponentType } from 'react';
import { SegmentedControl } from '../../../components/ui';
import { OfferedSecurityModes, SecurityMode } from '../../../domain';
import { ConnectionField, ConnectionFieldErrors } from '../model/ConnectionFieldErrors';
import type { ConnectionFormStepProps } from '../model/ConnectionFormStepProps';
import { MtlsFields } from './MtlsFields';
import { NoSecurityFields } from './NoSecurityFields';
import { OAuthFields } from './OAuthFields';
import { SaslCredentials } from './SaslCredentials';
import { ScramFields } from './ScramFields';
import { TlsOnlyFields } from './TlsOnlyFields';
import styles from './SecurityFields.module.css';

/** Fields shown for each security mode — only the chosen mode's fields appear (SecurityFormRegistry). */
const SecurityFormRegistry: Record<SecurityMode, ComponentType<ConnectionFormStepProps>> = {
  [SecurityMode.None]: NoSecurityFields,
  [SecurityMode.Tls]: TlsOnlyFields,
  [SecurityMode.SaslPlain]: SaslCredentials,
  [SecurityMode.SaslScram]: ScramFields,
  [SecurityMode.Mtls]: MtlsFields,
  [SecurityMode.OAuth]: OAuthFields,
};

/** Step 3 — segmented security mode plus that mode's fields. */
export function SecurityStep(props: ConnectionFormStepProps) {
  const { form, set, errors } = props;
  const Fields = SecurityFormRegistry[form.security];
  const modeError = ConnectionFieldErrors.of(errors, ConnectionField.Security);
  return (
    <>
      <SegmentedControl options={[...OfferedSecurityModes]} value={form.security} onChange={(mode) => set('security', mode)} ariaLabel="Security" />
      {modeError && <div className={styles.error}>{modeError}</div>}
      <Fields {...props} />
    </>
  );
}
