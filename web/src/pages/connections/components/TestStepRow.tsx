import type { ConnectionTestStep } from '../../../domain';
import { TestStepPresentation } from '../model/ConnectionDrawer';
import styles from './TestStepRow.module.css';

/** One check of the test checklist: tick, cross or pulsing dot, plus what happened and what to do. */
export function TestStepRow({ step }: { step: ConnectionTestStep }) {
  const look = TestStepPresentation[step.state];
  const message = step.message ?? step.reason;
  return (
    <div className={styles.row}>
      <span className={[styles.icon, look.pulse && styles.pulse].filter(Boolean).join(' ')} data-tone={look.tone}>
        {look.icon}
      </span>
      <div className={styles.body}>
        <div className={styles.head}>
          <span className={look.dim ? styles.dim : undefined}>{step.label}</span>
          {step.durationMs !== undefined && <span className={styles.duration}>{step.durationMs} ms</span>}
        </div>
        {message && (
          <div className={styles.reason}>
            <div>{message}</div>
            {step.hint && <div className={styles.hint}>{step.hint}</div>}
          </div>
        )}
      </div>
    </div>
  );
}
