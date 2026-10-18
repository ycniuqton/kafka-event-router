import { Button, ButtonSize, ButtonVariant } from '../../../../components/ui';
import { PatchOpKind, type TestCase, type TestCaseResult } from '../../../../domain';
import { useStudio } from '../hooks/StudioContext';
import { StudioText } from '../model/StudioText';
import type { TestCaseController } from './TestCaseController';
import { TestCaseText } from './TestCaseText';
import styles from './TestCasesPanel.module.css';

type TestCaseDiffProps = { testCase: TestCase; result: TestCaseResult; controller: TestCaseController };

/** Expected → actual of a failing case (RFC 6902 operations) and the actions that re-lock it. */
export function TestCaseDiff({ testCase, result, controller }: TestCaseDiffProps) {
  const { model } = useStudio();
  const accept = () => void controller.relock(testCase, { expectedOutcome: result.actualOutcome, expectedOutput: result.actualOutput });
  return (
    <div className={styles.diff}>
      {result.actualOutcome !== testCase.expectedOutcome && (
        <div className={styles.op} data-op={PatchOpKind.Replace}>
          <span className={styles.path}>{TestCaseText.outcome}</span> {testCase.expectedOutcome} → {result.actualOutcome}
        </div>
      )}
      {result.diff.map((op, i) => (
        <div key={`${op.path}:${i}`} className={styles.op} data-op={op.op}>
          <span className={styles.opName}>{op.op}</span> <span className={styles.path}>{op.path || '/'}</span>
          {op.value !== undefined && <span className={styles.value}> {StudioText.exampleValue(op.value)}</span>}
        </div>
      ))}
      {!model.readOnly && (
        <div className={styles.actions}>
          <Button size={ButtonSize.Sm} variant={ButtonVariant.Secondary} title={TestCaseText.acceptTitle} onClick={accept}>{TestCaseText.acceptActual}</Button>
        </div>
      )}
    </div>
  );
}
