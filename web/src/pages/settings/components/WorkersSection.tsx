import { services } from '../../../api';
import { useCan } from '../../../access/useCan';
import { Mono, ToneBadge } from '../../../components/ui';
import { Permission, RuntimeRefresh } from '../../../domain';
import { useNow, useServiceQuery } from '../../../store';
import { WorkerFleet, WorkerText } from '../model/WorkerFleet';
import { SettingsGridRow } from './SettingsGridRow';
import { SettingsSection } from './SettingsSection';
import styles from './SettingsSection.module.css';

/** Data-plane workers from `GET /workers` (workspace admins only; hidden for everyone else). */
export function WorkersSection() {
  const allowed = useCan(Permission.SettingsManage);
  if (!allowed) return null;
  return (
    <SettingsSection title={WorkerText.title} hint={WorkerText.hint}>
      <WorkerRows />
    </SettingsSection>
  );
}

/** Rows refreshed on the runtime polling cadence. */
function WorkerRows() {
  const now = useNow(RuntimeRefresh.Ms);
  const { data, error } = useServiceQuery(() => services.runtime.workers(), [now.getTime()]);
  if (!data) return <SettingsGridRow columns={WorkerFleet.messageColumns}><span className={styles.muted}>{error ? WorkerText.unavailable : WorkerText.loading}</span></SettingsGridRow>;
  if (data.length === 0) return <SettingsGridRow columns={WorkerFleet.messageColumns}><span className={styles.muted}>{WorkerText.empty}</span></SettingsGridRow>;
  return (
    <>
      {WorkerFleet.rows(data, now).map((w) => (
        <SettingsGridRow key={w.id} columns={WorkerFleet.columns}>
          <Mono size="sm" truncate>{w.id}</Mono>
          <span className={styles.mono}>{w.place}</span>
          <span><ToneBadge tone={w.tone} dot>{w.label}</ToneBadge></span>
          <span className={styles.secondary}>{w.load}</span>
          <span className={styles.muted}>{w.heartbeat}</span>
        </SettingsGridRow>
      ))}
    </>
  );
}
