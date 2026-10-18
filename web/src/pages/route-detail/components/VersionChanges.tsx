import { Card } from '../../../components/ui';
import { DiffMarkTone, type RouteVersion } from '../../../domain';
import styles from './VersionChanges.module.css';

/** "What changed in vN": the version's note and its +/−/~ change lines. */
export function VersionChanges({ version }: { version: RouteVersion }) {
  return (
    <Card padding="md" className={styles.panel}>
      <div className={styles.title}>What changed in v{version.version}</div>
      <div className={styles.note}>{version.note}</div>
      {version.diff.map((d, i) => (
        <div key={i} className={styles.line} data-tone={DiffMarkTone[d.mark]}>
          <span className={styles.mark}>{d.mark}</span>
          <span>{d.text}</span>
        </div>
      ))}
    </Card>
  );
}
