import { SegmentedControl, SegmentSize } from '../../../../components/ui';
import { StartPosition } from '../../../../domain';
import { StartFromOptions } from '../../model/SourceRules';
import styles from './StartFromField.module.css';

export type StartFromFieldProps = {
  value: StartPosition;
  at: string | undefined;
  onChange: (value: StartPosition) => void;
  onAt: (at: string) => void;
};

/** Start from: Latest · Earliest · Date / time (UTC) · Offsets — where a brand-new consumer group begins reading. */
export function StartFromField({ value, at, onChange, onAt }: StartFromFieldProps) {
  return (
    <div className={styles.field}>
      <span className={styles.label}>Start from</span>
      <div>
        <SegmentedControl options={StartFromOptions.positions} value={value} onChange={onChange} size={SegmentSize.Md} ariaLabel="Start from" />
      </div>
      {value === StartPosition.DateTime && (
        <label className={styles.dateRow}>
          <input
            type="datetime-local"
            className={styles.date}
            value={StartFromOptions.toInput(at)}
            onChange={(e) => {
              const iso = StartFromOptions.fromInput(e.target.value);
              if (iso) onAt(iso);
            }}
          />
          <span className={styles.hint}>UTC</span>
        </label>
      )}
      <span className={styles.hint}>Applies when the route's consumer group has no committed offsets yet.</span>
    </div>
  );
}
