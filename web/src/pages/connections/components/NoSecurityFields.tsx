import styles from './SecurityFields.module.css';

/** Warning shown when the connection has no authentication or encryption. */
export function NoSecurityFields() {
  return <div className={styles.warning}>No authentication or encryption. Use only for local development.</div>;
}
