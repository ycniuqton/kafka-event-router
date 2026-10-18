import { useState } from 'react';
import { services } from '../../../api';
import { accessStore, useCan } from '../../../access';
import { Button, ButtonSize, ButtonVariant, ToneBadge } from '../../../components/ui';
import { Permission, Tone, type TeamMember } from '../../../domain';
import { actions, useServiceQuery } from '../../../store';
import { MemberRoles } from '../model/MemberRoles';
import { AddToTeamDialog } from './AddToTeamDialog';
import { InviteDialog } from './InviteDialog';
import { RoleSelect } from './RoleSelect';
import { SettingsGridRow } from './SettingsGridRow';
import { SettingsSection } from './SettingsSection';
import styles from './SettingsSection.module.css';

const MemberColumns = '1fr 140px 120px 170px';

/** Members with one row per team role (admins edit roles, add to teams, remove, invite by email). */
export function MembersSection({ members, onChanged }: { members: TeamMember[]; onChanged: () => void }) {
  const { data: teams } = useServiceQuery(() => services.teams.list(), []);
  const canInvite = useCan(Permission.UserManage);
  const [inviting, setInviting] = useState(false);
  const [adding, setAdding] = useState<TeamMember | null>(null);
  const hint = (
    <>
      Admins can change roles. Approvers also have every Editor right in their teams.{' '}
      {canInvite && (
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => setInviting(true)}>
          + Invite
        </Button>
      )}
    </>
  );
  return (
    <SettingsSection title="Members & roles" hint={hint}>
      {members.map((m) => (
        <SettingsGridRow key={MemberRoles.key(m)} columns={MemberColumns}>
          <span>
            {m.name} {MemberRoles.isInvited(m) && <ToneBadge tone={Tone.Info}>Invited</ToneBadge>}
          </span>
          <span className={styles.accent}>
            <RoleSelect member={m} onChanged={onChanged} />
          </span>
          <span className={styles.muted}>{m.team}</span>
          <MemberActions member={m} members={members} onAdd={() => setAdding(m)} onChanged={onChanged} />
        </SettingsGridRow>
      ))}
      {inviting && <InviteDialog teams={teams ?? []} open onClose={() => setInviting(false)} onInvited={onChanged} />}
      {adding && (
        <AddToTeamDialog member={adding} teams={MemberRoles.otherTeams(adding, members, teams ?? [])} onClose={() => setAdding(null)} onChanged={onChanged} />
      )}
    </SettingsSection>
  );
}

type MemberActionsProps = { member: TeamMember; members: TeamMember[]; onAdd: () => void; onChanged: () => void };

/** Row actions: remove from this team; "Add to team" once per user (needs team.members_manage). */
function MemberActions({ member, members, onAdd, onChanged }: MemberActionsProps) {
  const canManage = useCan(Permission.TeamMembersManage, member.teamId);
  if (!canManage) return <span />;
  const remove = async () => {
    try {
      actions.toast.show(await MemberRoles.removeFromTeam(member));
      void accessStore.refresh();
      onChanged();
    } catch (error) {
      actions.toast.show((error as Error).message);
    }
  };
  return (
    <span className={styles.actions}>
      {MemberRoles.isFirstRow(member, members) && (
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={onAdd}>
          Add to team
        </Button>
      )}
      {member.teamId && (
        <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => void remove()}>
          Remove
        </Button>
      )}
    </span>
  );
}
