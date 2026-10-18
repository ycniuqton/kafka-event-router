import type { ConnectionTestRun } from '../../../domain';
import { TestStepRow } from '../components/TestStepRow';
import styles from './TestChecklist.module.css';

export type TestChecklistProps = { run: ConnectionTestRun | undefined };

/** Live checklist of a connection test (saved or draft): ticks, failure reason + hint, run errors. */
export function TestChecklist({ run }: TestChecklistProps) {
  if (!run) return null;
  return (
    <div className={styles.list} aria-live="polite">
      {run.steps.map((step) => (
        <TestStepRow key={step.label} step={step} />
      ))}
      {run.error && (
        <div className={styles.error} role="alert">
          {run.error}
        </div>
      )}
      {run.ok && <div className={styles.passed}>All checks passed.</div>}
    </div>
  );
}
