import { EnvMeta, type EnvTag as EnvTagValue } from '../../domain';
import styles from './EnvTag.module.css';

/** Outlined environment tag: prod magenta, staging blue, dev grey. */
export function EnvTag({ env }: { env: EnvTagValue }) {
  const meta = EnvMeta[env];
  return (
    <span className={styles.tag} data-tone={meta.tone}>
      {meta.label}
    </span>
  );
}
