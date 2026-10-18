import { describe, expect, it } from 'vitest';
import golden from '../../../testdata/rbac/matrix.golden.json';
import { AccessPolicy, Permission, RoleMatrix } from './access';
import { RoleKey } from './member';

describe('RoleMatrix', () => {
  it('equals the Go golden matrix', () => {
    const roles = Object.fromEntries(Object.entries(RoleMatrix.roles).map(([role, perms]) => [role, [...perms].sort()]));
    expect(roles).toEqual(golden.roles);
    expect([...RoleMatrix.workspaceAdmin].sort()).toEqual(golden.workspaceAdmin);
  });
});

describe('AccessPolicy', () => {
  const editorA = RoleMatrix.accessFor(false, { a: RoleKey.Editor });
  const admin = RoleMatrix.accessFor(true, {});

  it.each([
    ['nothing before access loads', null, Permission.RouteRead, 'a', false],
    ['own team write', editorA, Permission.RouteWrite, 'a', true],
    ['other team write', editorA, Permission.RouteWrite, 'b', false],
    ['other team read', editorA, Permission.RouteRead, 'b', true],
    ['other team samples', editorA, Permission.SampleRead, 'b', false],
    ['collection write uses any team', editorA, Permission.RouteWrite, undefined, true],
    ['admin-only on own team', editorA, Permission.ConnectionWrite, 'a', false],
    ['workspace permission', editorA, Permission.AuditRead, undefined, false],
    ['workspace admin everything', admin, Permission.UserManage, undefined, true],
  ] as const)('%s', (_, access, permission, team, want) => {
    expect(AccessPolicy.can(access, permission, team)).toBe(want);
  });

  it('names the lowest role of a permission', () => {
    expect(AccessPolicy.requiredRole(Permission.RouteWrite)).toBe(RoleKey.Editor);
    expect(AccessPolicy.requiredRole(Permission.DeploymentApprove)).toBe(RoleKey.Approver);
    expect(AccessPolicy.requiredRole(Permission.AuditRead)).toBeNull();
  });
});
