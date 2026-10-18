import type { ReactNode } from 'react';
import styles from './SettingsSection.module.css';

/** A 120px label + control row (design Settings → Theme). */
export function SettingRow({ label, children }: { label: ReactNode; children: ReactNode }) {
  return (
    <div className={styles.settingRow}>
      <span className={styles.settingLabel}>{label}</span>
      <div className={styles.settingControl}>{children}</div>
    </div>
  );
}
