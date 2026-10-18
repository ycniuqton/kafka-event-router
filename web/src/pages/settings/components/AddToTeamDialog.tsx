import { useState } from 'react';
import { accessStore } from '../../../access';
import { Button, ButtonVariant, Field, Modal, Select } from '../../../components/ui';
import { RoleKey, RoleKeys, type Team, type TeamMember } from '../../../domain';
import { actions } from '../../../store';
import { MemberRoles } from '../model/MemberRoles';

export type AddToTeamDialogProps = { member: TeamMember; teams: Team[]; onClose: () => void; onChanged: () => void };

/** Gives a member a role in one more team (users may hold one role per team). */
export function AddToTeamDialog({ member, teams, onClose, onChanged }: AddToTeamDialogProps) {
  const [teamId, setTeamId] = useState(teams[0]?.id ?? '');
  const [role, setRole] = useState<RoleKey>(RoleKey.Viewer);
  const team = teams.find((t) => t.id === teamId);
  const add = async () => {
    if (!team) return;
    try {
      actions.toast.show(await MemberRoles.addToTeam(member, team, role));
      void accessStore.refresh();
      onChanged();
      onClose();
    } catch (error) {
      actions.toast.show((error as Error).message);
    }
  };
  const footer = (
    <Button variant={ButtonVariant.Primary} disabled={!team} onClick={() => void add()}>
      Add to team
    </Button>
  );
  return (
    <Modal open onClose={onClose} title={`Add ${member.name} to a team`} footer={footer}>
      <Field label="Team">
        <Select block value={teamId} aria-label="Team" onChange={(e) => setTeamId(e.target.value)}>
          {teams.map((t) => (
            <option key={t.id} value={t.id}>
              {t.name}
            </option>
          ))}
        </Select>
      </Field>
      <Field label="Role">
        <Select block value={role} aria-label="Role" onChange={(e) => setRole(e.target.value as RoleKey)}>
          {RoleKeys.assignable.map((r) => (
            <option key={r} value={r}>
              {RoleKeys.toRole(r)}
            </option>
          ))}
        </Select>
      </Field>
    </Modal>
  );
}
