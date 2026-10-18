import type { Access, InviteDraft, RoleKey, TeamMember, UserPatch, WorkspaceSettings, WorkspaceSettingsPatch } from '../../../domain';
import { AccessService } from '../../contracts';
import { CursorIterator, PageLimit } from '../core/CursorIterator';
import type { HttpContext } from '../core/HttpContext';
import { AccessEndpoint, EntityTag, MemberMapper, WorkspaceSettingsMapper, type MeAccessDto, type RoleBindingDto, type UserDto, type WorkspaceSettingsDto } from './MemberDtos';

/** Member administration over `/users` and `/role-bindings`; effective access from `/auth/me`. */
export class HttpAccessService extends AccessService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async me(): Promise<Access> {
    return (await this.context.api.get<MeAccessDto>(AccessEndpoint.me)).access;
  }

  async members(): Promise<TeamMember[]> {
    const users = await new CursorIterator<UserDto>(this.context.api, AccessEndpoint.users, { limit: PageLimit.Max }).all();
    return MemberMapper.rows(users);
  }

  async setRole(member: TeamMember, role: RoleKey): Promise<TeamMember> {
    const teamId = member.teamId ?? '';
    const binding = await this.context.api.put<RoleBindingDto>(AccessEndpoint.binding(member.userId, teamId), { role }, {
      ifMatch: EntityTag.of(member.bindingVersion),
    });
    return MemberMapper.withBinding(member, binding);
  }

  async removeBinding(member: TeamMember): Promise<void> {
    await this.context.api.delete(AccessEndpoint.binding(member.userId, member.teamId ?? ''), { ifMatch: EntityTag.of(member.bindingVersion) });
  }

  async updateUser(member: TeamMember, patch: UserPatch): Promise<void> {
    await this.context.api.patch<UserDto>(AccessEndpoint.user(member.userId), patch, { ifMatch: EntityTag.of(member.userVersion) });
  }

  async invite(draft: InviteDraft): Promise<TeamMember[]> {
    const user = await this.context.api.post<UserDto>(AccessEndpoint.invitations, {
      email: draft.email.trim(), displayName: draft.displayName.trim(), teamId: draft.teamId, role: draft.role,
    });
    return MemberMapper.rows([user]);
  }

  async settings(): Promise<WorkspaceSettings> {
    return WorkspaceSettingsMapper.settings(await this.context.api.get<WorkspaceSettingsDto>(AccessEndpoint.settings));
  }

  async updateSettings(current: WorkspaceSettings, patch: WorkspaceSettingsPatch): Promise<WorkspaceSettings> {
    const dto = await this.context.api.patch<WorkspaceSettingsDto>(AccessEndpoint.settings, patch, { ifMatch: EntityTag.of(current.version) });
    return WorkspaceSettingsMapper.settings(dto);
  }
}
