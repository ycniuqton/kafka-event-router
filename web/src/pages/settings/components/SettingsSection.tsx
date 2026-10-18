import type { ReactNode } from 'react';
import { Card } from '../../../components/ui';
import styles from './SettingsSection.module.css';

/** Section caption followed by a card of rows. */
export function SettingsSection({ title, hint, children }: { title: ReactNode; hint?: ReactNode; children: ReactNode }) {
  return (
    <section className={styles.section}>
      <div className={styles.caption}>
        <span className={styles.title}>{title}</span>
        {hint && <span className={styles.hint}>{hint}</span>}
      </div>
      <Card>{children}</Card>
    </section>
  );
}
