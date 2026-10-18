import { useAccess } from './useCan';
import { AccessPolicy, Permission } from '../domain';
import { MaskedHintText } from './MaskedHintText';
import styles from './MaskedHint.module.css';

/** Banner shown above payload views (samples, live tail, DLQ) when the server masked the values. */
export function MaskedHint({ teamId }: { teamId?: string | null }) {
  const access = useAccess();
  const raw = AccessPolicy.can(access, Permission.SampleReadRaw, teamId);
  return (
    <div className={styles.hint} role="note">
      <span className={styles.lock} aria-hidden>
        ●
      </span>
      <span className={styles.title}>{MaskedHintText.Title}</span>
      <span className={styles.reason}>{raw ? MaskedHintText.Sensitive : MaskedHintText.Viewer}</span>
    </div>
  );
}
