/** Workspace roles (spec §11.2). */
export const Role = { Viewer: 'Viewer', Editor: 'Editor', Approver: 'Approver', Admin: 'Admin' } as const;
export type Role = (typeof Role)[keyof typeof Role];

export type Member = { name: string; role: Role; team: string };

/** The signed-in user and workspace shown in the shell. */
export type CurrentUser = Member & { initials: string; workspace: string };

/** API role keys (Postgres `team_role`); `Role` stays the display enum. */
export const RoleKey = { Viewer: 'viewer', Editor: 'editor', Approver: 'approver', Admin: 'admin' } as const;
export type RoleKey = (typeof RoleKey)[keyof typeof RoleKey];

/** Lifecycle of a user in Settings → Members. */
export const UserStatus = { Active: 'active', Invited: 'invited', Disabled: 'disabled' } as const;
export type UserStatus = (typeof UserStatus)[keyof typeof UserStatus];

/** Maps API role keys to display roles and back; lists the assignable roles (P2: all four). */
export class RoleKeys {
  private static readonly display: Readonly<Record<RoleKey, Role>> = {
    viewer: Role.Viewer,
    editor: Role.Editor,
    approver: Role.Approver,
    admin: Role.Admin,
  };
  static readonly ordered: readonly RoleKey[] = [RoleKey.Viewer, RoleKey.Editor, RoleKey.Approver, RoleKey.Admin];
  static readonly assignable: readonly RoleKey[] = [RoleKey.Viewer, RoleKey.Editor, RoleKey.Approver, RoleKey.Admin];

  static toRole(key: RoleKey): Role {
    return RoleKeys.display[key];
  }

  static fromRole(role: Role): RoleKey {
    return RoleKeys.ordered.find((k) => RoleKeys.display[k] === role) ?? RoleKey.Viewer;
  }
}

/** One member row in Settings: a user's binding in one team (team is null for users without a team). */
export type TeamMember = Member & {
  userId: string;
  email: string;
  teamId: string | null;
  roleKey: RoleKey | null;
  bindingVersion: number | null;
  status: UserStatus;
  workspaceAdmin: boolean;
  userVersion: number;
  /** The user has a password credential (password mode). */
  passwordLogin: boolean;
  /** The user must change their password at the next sign-in. */
  mustChangePassword: boolean;
};

/** Fields an admin may change on a user (`PATCH /users/{id}`). */
export type UserPatch = { displayName?: string; workspaceAdmin?: boolean; disabled?: boolean };
