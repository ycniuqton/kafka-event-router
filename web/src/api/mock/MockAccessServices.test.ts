import { describe, expect, it } from 'vitest';
import { RoleKey, type TeamMember } from '../../domain';
import { MockAccessService, MockAccessText } from './MockAccessServices';
import { MockBackend } from './MockBackend';

async function setup(): Promise<{ svc: MockAccessService; rows: TeamMember[] }> {
  const svc = new MockAccessService(new MockBackend());
  return { svc, rows: await svc.members() };
}

const byName = (rows: TeamMember[], name: string) => rows.find((r) => r.name === name) as TeamMember;

describe('MockAccessService', () => {
  it('changes a role and bumps the binding version', async () => {
    const { svc, rows } = await setup();
    const hoa = await svc.setRole(byName(rows, 'Hoa L.'), RoleKey.Admin);
    expect(hoa.roleKey).toBe(RoleKey.Admin);
    expect(hoa.bindingVersion).toBe(2);
    await expect(svc.setRole(byName(rows, 'Hoa L.'), RoleKey.Editor)).rejects.toThrow(MockAccessText.Stale);
  });

  it('assigns all four roles (P2) but never unknown ones', async () => {
    const { svc, rows } = await setup();
    await expect(svc.setRole(byName(rows, 'Hoa L.'), RoleKey.Approver)).resolves.toMatchObject({ roleKey: RoleKey.Approver });
    await expect(svc.setRole(byName(await svc.members(), 'Hoa L.'), 'owner' as RoleKey)).rejects.toThrow(MockAccessText.NotAssignable);
  });

  it('invites by email once, as a pending member of one team', async () => {
    const { svc } = await setup();
    const [row] = await svc.invite({ email: 'Lan@vcc.internal', displayName: '', teamId: 'team-payments', role: RoleKey.Viewer });
    expect(row).toMatchObject({ email: 'lan@vcc.internal', name: 'lan', status: 'invited', roleKey: RoleKey.Viewer, team: 'Payments' });
    await expect(svc.invite({ email: 'lan@vcc.internal', displayName: '', teamId: 'team-payments', role: RoleKey.Viewer })).rejects.toThrow(
      MockAccessText.UserExists,
    );
    await expect(svc.invite({ email: 'nope', displayName: '', teamId: 'team-payments', role: RoleKey.Viewer })).rejects.toThrow(MockAccessText.InvalidEmail);
  });

  it('never removes the last administrator', async () => {
    const { svc, rows } = await setup();
    await expect(svc.updateUser(byName(rows, 'Quincy N.'), { disabled: true })).rejects.toThrow(MockAccessText.LastAdmin);
    await svc.updateUser(byName(rows, 'Minh N.'), { workspaceAdmin: true });
    await expect(svc.updateUser(byName(rows, 'Quincy N.'), { workspaceAdmin: false })).resolves.toBeUndefined();
  });

  it('derives the caller access from the matrix', async () => {
    const { svc } = await setup();
    const access = await svc.me();
    expect(access.workspaceAdmin).toBe(true);
    expect(access.workspace).toContain('user.manage');
  });
});
