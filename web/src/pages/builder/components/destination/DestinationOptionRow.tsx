import type { ReactNode } from 'react';
import { SegmentedControl, SegmentSize } from '../../../../components/ui';
import styles from './DestinationOptionRow.module.css';

export type DestinationOptionRowProps = {
  label: string;
  options: readonly string[];
  value: string;
  onChange: (value: string) => void;
  /** Extra input revealed by the current choice (key field, Avro subject, DLQ name…). */
  children?: ReactNode;
};

/** A labelled segmented choice of the destination options list. */
export function DestinationOptionRow({ label, options, value, onChange, children }: DestinationOptionRowProps) {
  return (
    <div className={styles.option}>
      <div className={styles.row}>
        <span className={styles.label}>{label}</span>
        <SegmentedControl options={options} value={value} onChange={onChange} size={SegmentSize.Md} ariaLabel={label} />
      </div>
      {children && <div className={styles.extra}>{children}</div>}
    </div>
  );
}
