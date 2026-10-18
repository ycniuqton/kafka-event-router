import { ConnectionFormStepLabels } from '../model/ConnectionForm';
import styles from './FormStepper.module.css';

/** Progress state of one stepper segment. */
const SegmentState = { Done: 'done', Current: 'current', Todo: 'todo' } as const;

export type FormStepperProps = { step: number; onSelect: (step: number) => void };

/** The 4 clickable progress bars at the top of the connection panel. */
export function FormStepper({ step, onSelect }: FormStepperProps) {
  const stateOf = (i: number) => (i === step ? SegmentState.Current : i < step ? SegmentState.Done : SegmentState.Todo);
  return (
    <div className={styles.stepper}>
      {ConnectionFormStepLabels.map((label, i) => (
        <button key={label} type="button" className={styles.segment} data-state={stateOf(i)} onClick={() => onSelect(i)}>
          <span className={styles.bar} />
          <span className={styles.label}>
            {i + 1}. {label}
          </span>
        </button>
      ))}
    </div>
  );
}
