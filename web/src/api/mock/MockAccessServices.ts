import { RoleKey, RoleKeys, RoleMatrix, UserStatus, type Access, type InviteDraft, type TeamMember, type UserPatch, type WorkspaceSettings, type WorkspaceSettingsPatch } from '../../domain';
import { CURRENT_USER_ID, MEMBER_DIRECTORY, TEAMS } from '../../mocks';
import { AccessService } from '../contracts';
import type { MockBackend } from './MockBackend';

/** Workspace settings of a fresh workspace (AI assist off, production approval on for new teams). */
const MockWorkspaceDefaults: WorkspaceSettings = { aiAssistEnabled: false, defaultRequireProdApproval: true, version: 1 };

/** Messages of the mock access rules (same wording as the API problems). */
export const MockAccessText = {
  LastAdmin: 'This would leave no administrator. Make someone else an admin first, then retry.',
  NotAssignable: "This role can't be assigned in this workspace. Pick one of the offered roles.",
  UserExists: 'Someone with this email is already a member. Change their role in the member list instead.',
  InvalidEmail: 'Enter a valid email address, like name@company.com.',
  Stale: 'Someone else changed this. Reload to see their version, then retry.',
  NotFound: 'The requested resource does not exist. It may have been deleted.',
} as const;

/** Shape check of invitation emails in the mock (the server parses addresses properly). */
class MockEmail {
  static readonly pattern = /^[^\s@<>]+@[^\s@<>]+\.[^\s@<>]+$/;
}

/** In-memory member administration enforcing the server rules: assignable roles, If-Match versions, last admin, invitations. */
export class MockAccessService extends AccessService {
  private readonly backend: MockBackend;
  private directory: TeamMember[] = structuredClone(MEMBER_DIRECTORY);
  private workspace: WorkspaceSettings = { ...MockWorkspaceDefaults };

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  me(): Promise<Access> {
    const mine = this.directory.filter((m) => m.userId === CURRENT_USER_ID);
    const bindings: Record<string, RoleKey> = {};
    mine.forEach((m) => {
      if (m.teamId && m.roleKey) bindings[m.teamId] = m.roleKey;
    });
    return this.backend.reply(RoleMatrix.accessFor(mine.some((m) => m.workspaceAdmin), bindings));
  }

  members(): Promise<TeamMember[]> {
    return this.backend.reply(this.directory);
  }

  setRole(member: TeamMember, role: RoleKey): Promise<TeamMember> {
    const row = this.find(member);
    const problem = this.roleProblem(row, member, role);
    if (problem) return this.backend.fail(problem);
    if (!row) return this.backend.fail(MockAccessText.NotFound);
    row.roleKey = role;
    row.role = RoleKeys.toRole(role);
    row.bindingVersion = (row.bindingVersion ?? 0) + 1;
    return this.backend.reply(row);
  }

  removeBinding(member: TeamMember): Promise<void> {
    const row = this.find(member);
    if (!row) return this.backend.fail(MockAccessText.NotFound);
    if (row.bindingVersion !== member.bindingVersion) return this.backend.fail(MockAccessText.Stale);
    if (this.isLastAdmin(row)) return this.backend.fail(MockAccessText.LastAdmin);
    this.directory = this.directory.filter((m) => m !== row);
    return this.backend.reply(undefined);
  }

  updateUser(member: TeamMember, patch: UserPatch): Promise<void> {
    const rows = this.directory.filter((m) => m.userId === member.userId);
    if (rows.length === 0) return this.backend.fail(MockAccessText.NotFound);
    if (rows[0].userVersion !== member.userVersion) return this.backend.fail(MockAccessText.Stale);
    const losesAdmin = patch.workspaceAdmin === false || patch.disabled === true;
    if (losesAdmin && rows[0].workspaceAdmin && this.otherWorkspaceAdmins(member.userId) === 0) return this.backend.fail(MockAccessText.LastAdmin);
    rows.forEach((r) => this.applyPatch(r, patch));
    return this.backend.reply(undefined);
  }

  invite(draft: InviteDraft): Promise<TeamMember[]> {
    const email = draft.email.trim().toLowerCase();
    if (!MockEmail.pattern.test(email)) return this.backend.fail(MockAccessText.InvalidEmail);
    if (this.directory.some((m) => m.email.toLowerCase() === email)) return this.backend.fail(MockAccessText.UserExists);
    const role = draft.role as RoleKey;
    if (!RoleKeys.assignable.includes(role)) return this.backend.fail(MockAccessText.NotAssignable);
    const team = TEAMS.find((t) => t.id === draft.teamId);
    const row: TeamMember = {
      name: draft.displayName.trim() || email.split('@')[0], role: RoleKeys.toRole(role), team: team?.name ?? draft.teamId, userId: `user-${email}`,
      email, teamId: draft.teamId, roleKey: role, bindingVersion: 1, status: UserStatus.Invited, workspaceAdmin: false, userVersion: 1,
    };
    this.directory = [...this.directory, row];
    return this.backend.reply([row]);
  }

  settings(): Promise<WorkspaceSettings> {
    return this.backend.reply({ ...this.workspace });
  }

  /** Same rules as `PATCH /workspace-settings`: If-Match on the version, unchanged values keep it. */
  updateSettings(current: WorkspaceSettings, patch: WorkspaceSettingsPatch): Promise<WorkspaceSettings> {
    if (current.version !== this.workspace.version) return this.backend.fail(MockAccessText.Stale);
    const next = { ...this.workspace, ...patch };
    const changed = next.aiAssistEnabled !== this.workspace.aiAssistEnabled || next.defaultRequireProdApproval !== this.workspace.defaultRequireProdApproval;
    this.workspace = changed ? { ...next, version: this.workspace.version + 1 } : this.workspace;
    return this.backend.reply({ ...this.workspace });
  }

  private find(member: TeamMember): TeamMember | undefined {
    return this.directory.find((m) => m.userId === member.userId && m.teamId === member.teamId);
  }

  private roleProblem(row: TeamMember | undefined, member: TeamMember, role: RoleKey): string | null {
    if (!RoleKeys.assignable.includes(role)) return MockAccessText.NotAssignable;
    if (row && row.bindingVersion !== member.bindingVersion) return MockAccessText.Stale;
    if (row && role !== RoleKey.Admin && this.isLastAdmin(row)) return MockAccessText.LastAdmin;
    return null;
  }

  private isLastAdmin(row: TeamMember): boolean {
    if (row.roleKey !== RoleKey.Admin || row.workspaceAdmin) return false;
    const teamAdmins = this.directory.filter((m) => m !== row && m.teamId === row.teamId && m.roleKey === RoleKey.Admin && m.status === UserStatus.Active);
    return teamAdmins.length + this.otherWorkspaceAdmins(row.userId) === 0;
  }

  private otherWorkspaceAdmins(userId: string): number {
    const ids = new Set(this.directory.filter((m) => m.userId !== userId && m.workspaceAdmin && m.status === UserStatus.Active).map((m) => m.userId));
    return ids.size;
  }

  private applyPatch(row: TeamMember, patch: UserPatch): void {
    if (patch.displayName !== undefined) row.name = patch.displayName;
    if (patch.workspaceAdmin !== undefined) row.workspaceAdmin = patch.workspaceAdmin;
    if (patch.disabled !== undefined) row.status = patch.disabled ? UserStatus.Disabled : UserStatus.Active;
    row.userVersion += 1;
  }
}
