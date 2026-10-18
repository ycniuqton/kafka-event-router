import { Role } from '../../../domain';
import { RoleCapabilities } from '../model/SettingsCatalog';
import { SettingsGridRow } from './SettingsGridRow';
import { SettingsSection } from './SettingsSection';
import styles from './SettingsSection.module.css';

const RoleColumns = '120px minmax(0,1fr)';

/** What each role can do (spec §11.2); read-only. */
export function RolesSection() {
  return (
    <SettingsSection title="Roles" hint="Production deploys can require an Approver.">
      {Object.values(Role).map((role) => (
        <SettingsGridRow key={role} columns={RoleColumns}>
          <span className={styles.accent}>{role}</span>
          <span className={styles.secondary}>{RoleCapabilities[role]}</span>
        </SettingsGridRow>
      ))}
    </SettingsSection>
  );
}
