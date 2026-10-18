import styles from './HistoryToolbar.module.css';

/** Compare hint above the version list (YAML export/import arrive with routes#p2-fe-yaml). */
export function HistoryToolbar() {
  return (
    <div className={styles.toolbar}>
      <span className={styles.hint}>Tick two versions to compare them side by side.</span>
    </div>
  );
}
