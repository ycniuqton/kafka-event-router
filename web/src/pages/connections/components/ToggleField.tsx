import type { ReactNode } from 'react';
import styles from './ToggleField.module.css';

export type ToggleFieldProps = { label: ReactNode; hint?: ReactNode; checked: boolean; onChange: (checked: boolean) => void };

/** Checkbox with a label and a one-line explanation (panel opt-ins such as topic creation). */
export function ToggleField({ label, hint, checked, onChange }: ToggleFieldProps) {
  return (
    <label className={styles.toggle}>
      <input type="checkbox" className={styles.box} checked={checked} onChange={(e) => onChange(e.target.checked)} />
      <span className={styles.text}>
        <span>{label}</span>
        {hint && <span className={styles.hint}>{hint}</span>}
      </span>
    </label>
  );
}
