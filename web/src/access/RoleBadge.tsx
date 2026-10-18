import { ToneBadge } from '../components/ui';
import { RoleKey, RoleKeys, Tone } from '../domain';
import styles from './RoleBadge.module.css';

/** Tone per role: admins stand out, viewers are muted. */
const RoleTone: Readonly<Record<RoleKey, Tone>> = {
  [RoleKey.Viewer]: Tone.Off,
  [RoleKey.Editor]: Tone.Info,
  [RoleKey.Approver]: Tone.Warn,
  [RoleKey.Admin]: Tone.Accent,
};

/** A role pill ("Editor"); `workspaceAdmin` adds a "Workspace admin" marker. */
export function RoleBadge({ role, workspaceAdmin = false }: { role: RoleKey; workspaceAdmin?: boolean }) {
  return (
    <span className={styles.root}>
      <ToneBadge tone={RoleTone[role]} dot={false}>
        {RoleKeys.toRole(role)}
      </ToneBadge>
      {workspaceAdmin && <span className={styles.marker}>Workspace admin</span>}
    </span>
  );
}
