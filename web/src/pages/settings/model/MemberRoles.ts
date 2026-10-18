import { services } from '../../../api';
import { RoleKey as RoleKeyValues, RoleKeys, UserStatus, type InviteDraft, type RoleKey, type Team, type TeamMember } from '../../../domain';

/** Options of the role picker: the assignable roles, plus the current one shown disabled when it is not assignable. */
export type RoleOption = { value: RoleKey; label: string; disabled: boolean };

/** Role changes from Settings → Members: options, the change itself and its confirmation text. */
export class MemberRoles {
  static options(current: RoleKey | null): RoleOption[] {
    const keys = RoleKeys.ordered.filter((k) => RoleKeys.assignable.includes(k) || k === current);
    return keys.map((k) => ({ value: k, label: RoleKeys.toRole(k), disabled: !RoleKeys.assignable.includes(k) }));
  }

  /** Applies the change; resolves with the toast text or rejects with the server's message. */
  static async change(member: TeamMember, role: RoleKey): Promise<string> {
    const updated = await services.access.setRole(member, role);
    return `${updated.name} is now ${updated.role} in ${updated.team}`;
  }

  /** Adds the user to another team with a role (a new binding, no If-Match). */
  static async addToTeam(member: TeamMember, team: Team, role: RoleKey): Promise<string> {
    const updated = await services.access.setRole({ ...member, teamId: team.id, team: team.name, bindingVersion: null }, role);
    return `${updated.name} is now ${updated.role} in ${team.name}`;
  }

  /** Removes the user from the row's team (If-Match: the binding version). */
  static async removeFromTeam(member: TeamMember): Promise<string> {
    await services.access.removeBinding(member);
    return `${member.name} was removed from ${member.team}`;
  }

  /** Teams the user is not a member of yet. */
  static otherTeams(member: TeamMember, rows: readonly TeamMember[], teams: readonly Team[]): Team[] {
    const bound = new Set(rows.filter((r) => r.userId === member.userId).map((r) => r.teamId));
    return teams.filter((t) => !bound.has(t.id));
  }

  /** One row per user (the first of their team rows), for user-level actions. */
  static isFirstRow(member: TeamMember, rows: readonly TeamMember[]): boolean {
    return rows.find((r) => r.userId === member.userId) === member;
  }

  static blankInvite(teams: readonly Team[]): InviteDraft {
    return { email: '', displayName: '', teamId: teams[0]?.id ?? '', role: RoleKeyValues.Viewer };
  }

  static async invite(draft: InviteDraft): Promise<string> {
    const rows = await services.access.invite(draft);
    return `Invitation saved for ${rows[0]?.email ?? draft.email} — they get access at first sign-in`;
  }

  static isInvited(member: TeamMember): boolean {
    return member.status === UserStatus.Invited;
  }

  static key(member: TeamMember): string {
    return `${member.userId}:${member.teamId ?? ''}`;
  }
}
