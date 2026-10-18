import { Role, RoleKey, UserStatus, type CurrentUser, type Member, type TeamMember } from '../domain';

/** Mock team ids of the design's teams. */
export const MockTeamId = { Platform: 'team-platform', Payments: 'team-payments', Logistics: 'team-logistics', Leadership: 'team-leadership' } as const;

/** Settings → Members & roles (design `members`) with the ids and versions the access API returns. */
export const MEMBER_DIRECTORY: TeamMember[] = [
  {
    name: 'Quincy N.', role: Role.Admin, team: 'Platform', userId: 'user-quincy', email: 'quincy@vcc.internal',
    teamId: MockTeamId.Platform, roleKey: RoleKey.Admin, bindingVersion: 1, status: UserStatus.Active, workspaceAdmin: true, userVersion: 1,
  },
  {
    name: 'Minh N.', role: Role.Approver, team: 'Payments', userId: 'user-minh', email: 'minh@vcc.internal',
    teamId: MockTeamId.Payments, roleKey: RoleKey.Approver, bindingVersion: 1, status: UserStatus.Active, workspaceAdmin: false, userVersion: 1,
  },
  {
    name: 'Hoa L.', role: Role.Editor, team: 'Logistics', userId: 'user-hoa', email: 'hoa@vcc.internal',
    teamId: MockTeamId.Logistics, roleKey: RoleKey.Editor, bindingVersion: 1, status: UserStatus.Active, workspaceAdmin: false, userVersion: 1,
  },
  {
    name: 'Thu H.', role: Role.Viewer, team: 'Leadership', userId: 'user-thu', email: 'thu@vcc.internal',
    teamId: MockTeamId.Leadership, roleKey: RoleKey.Viewer, bindingVersion: 1, status: UserStatus.Active, workspaceAdmin: false, userVersion: 1,
  },
];

export const MEMBERS: Member[] = MEMBER_DIRECTORY.map(({ name, role, team }) => ({ name, role, team }));

/** The mock signed-in user (a workspace admin). */
export const CURRENT_USER_ID = 'user-quincy';

export const CURRENT_USER: CurrentUser = {
  name: 'Quincy N.',
  role: Role.Admin,
  team: 'Platform',
  initials: 'QN',
  workspace: 'vcc-platform',
};
