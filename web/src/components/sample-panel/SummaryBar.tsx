import styles from './SummaryBar.module.css';

export type SummaryBarProps = { summary: string; notices: string[]; busy: string | null; error: string | null; tip?: string };

/** Counts line ("500 loaded · 37 pass · 461 filtered · 2 errors"), limit notices, progress and run errors. */
export function SummaryBar({ summary, notices, busy, error, tip }: SummaryBarProps) {
  return (
    <div className={styles.bar}>
      <div className={styles.counts} data-testid="sample-summary">
        {summary}
        {busy && <span className={styles.busy}> · {busy}</span>}
      </div>
      {notices.map((n) => (
        <div key={n} className={styles.notice}>
          {n}
        </div>
      ))}
      {error && (
        <div className={styles.error} role="alert">
          {error}
        </div>
      )}
      {tip && <div className={styles.tip}>{tip}</div>}
    </div>
  );
}
