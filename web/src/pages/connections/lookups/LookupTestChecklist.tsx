import type { LookupTestResult } from '../../../domain';
import { LookupTestSteps } from './model/LookupTestSteps';
import { TestStepRow } from '../components/TestStepRow';
import styles from '../test/TestChecklist.module.css';

export type LookupTestChecklistProps = { result: LookupTestResult | null; failure?: string | null };

/** Result of a lookup source test: reachable → authenticated → sample, with the failure reason and hint. */
export function LookupTestChecklist({ result, failure }: LookupTestChecklistProps) {
  if (!result && !failure) return null;
  return (
    <div className={styles.list} aria-live="polite">
      {result && LookupTestSteps.rows(result).map((step) => <TestStepRow key={step.label} step={step} />)}
      {failure && (
        <div className={styles.error} role="alert">
          {failure}
        </div>
      )}
      {result?.ok && <div className={styles.passed}>All checks passed.</div>}
    </div>
  );
}
