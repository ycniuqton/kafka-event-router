import { ControlSize, Select } from '../../../../components/ui';
import type { FilterOp } from '../../../../domain';
import styles from './FilterRowEditor.module.css';

export type OperatorSelectProps = {
  op: FilterOp;
  ops: FilterOp[];
  onChange: (op: FilterOp) => void;
};

/** Type-aware operator dropdown; keeps a stored operator selectable even when the type changed. */
export function OperatorSelect({ op, ops, onChange }: OperatorSelectProps) {
  const shown = ops.includes(op) ? ops : [op, ...ops];
  return (
    <Select controlSize={ControlSize.Md} sunken className={styles.op} value={op} onChange={(e) => onChange(e.target.value as FilterOp)} aria-label="Operator">
      {shown.map((o) => (
        <option key={o} value={o}>
          {o}
        </option>
      ))}
    </Select>
  );
}
