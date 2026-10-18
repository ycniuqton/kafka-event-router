import { useState } from 'react';
import { RoleBadge, accessStore, useCan } from '../../../access';
import { Select } from '../../../components/ui';
import { Permission, type RoleKey, type TeamMember } from '../../../domain';
import { actions } from '../../../store';
import { MemberRoles } from '../model/MemberRoles';

export type RoleSelectProps = { member: TeamMember; onChanged: () => void };

/** Role picker for admins (all four roles); a read-only badge for everyone else. */
export function RoleSelect({ member, onChanged }: RoleSelectProps) {
  const canManage = useCan(Permission.TeamMembersManage, member.teamId);
  const [busy, setBusy] = useState(false);
  if (!member.roleKey) return <span>—</span>;
  if (!canManage || !member.teamId) return <RoleBadge role={member.roleKey} workspaceAdmin={member.workspaceAdmin} />;
  const change = async (role: RoleKey) => {
    setBusy(true);
    try {
      actions.toast.show(await MemberRoles.change(member, role));
      void accessStore.refresh();
      onChanged();
    } catch (error) {
      actions.toast.show((error as Error).message);
    } finally {
      setBusy(false);
    }
  };
  return (
    <Select controlSize="sm" value={member.roleKey} disabled={busy} aria-label={`Role of ${member.name}`} onChange={(e) => void change(e.target.value as RoleKey)}>
      {MemberRoles.options(member.roleKey).map((o) => (
        <option key={o.value} value={o.value} disabled={o.disabled}>
          {o.label}
        </option>
      ))}
    </Select>
  );
}
