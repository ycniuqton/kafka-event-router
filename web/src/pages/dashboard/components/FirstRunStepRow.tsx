import { Button } from '../../../components/ui';
import { Tone } from '../../../domain';
import type { FirstRunStep } from '../model/FirstRun';
import styles from './FirstRunStepRow.module.css';

export type FirstRunStepRowProps = { step: FirstRunStep; index: number; done: boolean; current: boolean; onAct: () => void };

/** Numbered step; ticked when done, primary action on the current step. */
export function FirstRunStepRow({ step, index, done, current, onAct }: FirstRunStepRowProps) {
  return (
    <li className={styles.row} data-tone={done ? Tone.Ok : undefined} aria-current={current ? 'step' : undefined}>
      <span className={styles.badge}>{done ? '✓' : index + 1}</span>
      <div className={styles.text}>
        <div className={styles.title}>{step.title}</div>
        <div className={styles.description}>{step.description}</div>
      </div>
      {!done && (
        <Button size="sm" variant={current ? 'primary' : 'secondary'} onClick={onAct}>
          {step.actionLabel}
        </Button>
      )}
    </li>
  );
}
