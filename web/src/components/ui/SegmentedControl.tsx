import type { ReactNode } from 'react';
import { SegmentSize } from './variants';
import styles from './SegmentedControl.module.css';

export type SegmentOption<T extends string> = { value: T; label: ReactNode };

export type SegmentedControlProps<T extends string> = {
  options: readonly SegmentOption<T>[] | readonly T[];
  value: T;
  onChange: (value: T) => void;
  size?: SegmentSize;
  mono?: boolean;
  ariaLabel?: string;
};

/** Pill group for exclusive choices (Table/Cards, 15m/1h/24h/7d, destination options). */
export function SegmentedControl<T extends string>({ options, value, onChange, size = 'sm', mono, ariaLabel }: SegmentedControlProps<T>) {
  const normalized = options.map((o) => (typeof o === 'string' ? { value: o, label: o } : o)) as SegmentOption<T>[];
  return (
    <div className={[styles.group, styles[size], mono && styles.mono].filter(Boolean).join(' ')} role="radiogroup" aria-label={ariaLabel}>
      {normalized.map((o) => (
        <button
          key={o.value}
          type="button"
          role="radio"
          aria-checked={o.value === value}
          className={[styles.option, o.value === value && styles.selected].filter(Boolean).join(' ')}
          onClick={() => onChange(o.value)}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}
