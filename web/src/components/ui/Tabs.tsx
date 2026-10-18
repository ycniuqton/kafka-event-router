import type { ReactNode } from 'react';
import styles from './Tabs.module.css';

export type TabItem<T extends string> = { id: T; label: ReactNode };
export type TabsProps<T extends string> = { items: TabItem<T>[]; value: T; onChange: (id: T) => void };

/** Underlined tab bar (route detail). */
export function Tabs<T extends string>({ items, value, onChange }: TabsProps<T>) {
  return (
    <div className={styles.tabs} role="tablist">
      {items.map((item) => (
        <button
          key={item.id}
          type="button"
          role="tab"
          aria-selected={item.id === value}
          className={[styles.tab, item.id === value && styles.active].filter(Boolean).join(' ')}
          onClick={() => onChange(item.id)}
        >
          {item.label}
        </button>
      ))}
    </div>
  );
}
