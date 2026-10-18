import type { ConnectionFormStepProps } from '../model/ConnectionFormStepProps';
import { TlsFields } from './TlsFields';
import styles from './SecurityFields.module.css';

/** TLS without client authentication (SSL): encryption plus server trust settings. */
export function TlsOnlyFields(props: ConnectionFormStepProps) {
  return (
    <>
      <div className={styles.note}>Encrypted with TLS; the broker does not authenticate the client.</div>
      <TlsFields {...props} />
    </>
  );
}
