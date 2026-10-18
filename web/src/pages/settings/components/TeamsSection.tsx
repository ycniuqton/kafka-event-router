import { useState } from 'react';
import { services } from '../../../api';
import { useCan } from '../../../access';
import { Button, ButtonSize, ButtonVariant } from '../../../components/ui';
import { Permission, type Team } from '../../../domain';
import { useServiceQuery } from '../../../store';
import { ApprovalPolicyLabel } from '../model/TeamAdmin';
import { GrantsEditor } from './GrantsEditor';
import { SettingsGridRow } from './SettingsGridRow';
import { SettingsSection } from './SettingsSection';
import { TeamEditor } from './TeamEditor';
import styles from './SettingsSection.module.css';

const TeamColumns = '1fr 210px 90px 150px';

/** Dialogs of the section. */
const TeamDialogKind = { Edit: 'edit', Grants: 'grants' } as const;

/** Which dialog is open: the editor (team or null for "new") or the grants of one team. */
type TeamDialog = { kind: typeof TeamDialogKind.Edit; team: Team | null } | { kind: typeof TeamDialogKind.Grants; team: Team } | null;

/** Teams that own routes and connections: approval policy, members, connection grants (admins edit). */
export function TeamsSection({ onChanged }: { onChanged?: () => void }) {
  const { data: teams, reload } = useServiceQuery(() => services.teams.list(), []);
  const canManage = useCan(Permission.TeamManage);
  const canGrant = useCan(Permission.GrantManage);
  const [dialog, setDialog] = useState<TeamDialog>(null);
  const saved = () => {
    reload();
    onChanged?.();
  };
  const hint = (
    <>
      Routes and connections belong to a team.{' '}
      {canManage && (
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => setDialog({ kind: TeamDialogKind.Edit, team: null })}>
          + New team
        </Button>
      )}
    </>
  );
  return (
    <SettingsSection title="Teams" hint={hint}>
      {(teams ?? []).map((t) => (
        <SettingsGridRow key={t.id} columns={TeamColumns}>
          <span>{t.name}</span>
          <span className={styles.secondary}>{t.requireProdApproval ? ApprovalPolicyLabel.Required : ApprovalPolicyLabel.Direct}</span>
          <span className={styles.muted}>
            {t.memberCount} {t.memberCount === 1 ? 'member' : 'members'}
          </span>
          <span className={styles.actions}>
            <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => setDialog({ kind: TeamDialogKind.Grants, team: t })}>
              Grants
            </Button>
            {canManage && (
              <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => setDialog({ kind: TeamDialogKind.Edit, team: t })}>
                Edit
              </Button>
            )}
          </span>
        </SettingsGridRow>
      ))}
      {dialog?.kind === TeamDialogKind.Edit && <TeamEditor key={dialog.team?.id ?? 'new'} team={dialog.team} open onClose={() => setDialog(null)} onSaved={saved} />}
      {dialog?.kind === TeamDialogKind.Grants && <GrantsEditor team={dialog.team} open canManage={canGrant} onClose={() => setDialog(null)} />}
    </SettingsSection>
  );
}
