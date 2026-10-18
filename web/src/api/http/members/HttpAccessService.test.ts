import { describe, expect, it } from 'vitest';
import { RoleKey, UserStatus } from '../../../domain';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import { HttpAccessService } from './HttpAccessService';
import { MemberMapper } from './MemberDtos';

const user = {
  id: 'u1', email: 'hoa@x.io', displayName: 'Hoa L.', workspaceAdmin: false, status: UserStatus.Active, rowVersion: 3,
  teams: [{ teamId: 't1', teamName: 'Logistics', role: RoleKey.Editor, rowVersion: 2 }],
};

function service(fetch: TestFetch): HttpAccessService {
  return new HttpAccessService(new HttpContext({ apiBaseUrl: '/api/v1', authBaseUrl: '', fetch: fetch.fn }));
}

describe('HttpAccessService', () => {
  it('lists members across pages, one row per binding', async () => {
    const fetch = new TestFetch()
      .json(200, { items: [user], next_cursor: 'c2' })
      .json(200, { items: [{ ...user, id: 'u2', teams: [] }], next_cursor: null });
    const rows = await service(fetch).members();
    expect(rows.map((r) => [r.userId, r.teamId, r.roleKey, r.bindingVersion])).toEqual([
      ['u1', 't1', RoleKey.Editor, 2],
      ['u2', null, null, null],
    ]);
    expect(fetch.calls[1].url).toContain('cursor=c2');
  });

  it('sends If-Match with the binding version and maps the result', async () => {
    const fetch = new TestFetch().json(200, { userId: 'u1', teamId: 't1', teamName: 'Logistics', role: RoleKey.Admin, rowVersion: 3 });
    const [hoa] = MemberMapper.rows([user]);
    const updated = await service(fetch).setRole(hoa, RoleKey.Admin);
    expect(updated.roleKey).toBe(RoleKey.Admin);
    expect(updated.bindingVersion).toBe(3);
    expect(fetch.calls[0].init.method).toBe('PUT');
    expect(fetch.header(0, 'If-Match')).toBe('"2"');
    expect(JSON.parse(String(fetch.calls[0].init.body))).toEqual({ role: RoleKey.Admin });
  });

  it('reads access from /auth/me and surfaces last-admin problems', async () => {
    const fetch = new TestFetch()
      .json(200, { id: 'u1', access: { workspaceAdmin: false, workspace: [], teams: {} } })
      .problem(409, { type: 'https://eventrouter.dev/problems/conflict', title: 'Conflict', status: 409, code: 'last_admin', detail: 'This would leave no administrator.' });
    const svc = service(fetch);
    expect((await svc.me()).workspaceAdmin).toBe(false);
    const member = { ...MemberMapper.rows([user])[0], bindingVersion: 1 };
    const error = await TestFetch.failure(svc.removeBinding(member));
    expect(error.message).toBe('This would leave no administrator.');
    expect(fetch.header(1, 'If-Match')).toBe('"1"');
    expect(fetch.calls[1].url).toBe('/api/v1/role-bindings/u1/t1');
  });

  it('reads and patches workspace settings with the version as If-Match', async () => {
    const fetch = new TestFetch()
      .json(200, { aiAssistEnabled: false, defaultRequireProdApproval: true, rowVersion: 4 })
      .json(200, { aiAssistEnabled: true, defaultRequireProdApproval: true, rowVersion: 5 });
    const svc = service(fetch);
    const current = await svc.settings();
    expect(current).toEqual({ aiAssistEnabled: false, defaultRequireProdApproval: true, version: 4 });
    expect(await svc.updateSettings(current, { aiAssistEnabled: true })).toMatchObject({ aiAssistEnabled: true, version: 5 });
    expect(fetch.calls[1].url).toBe('/api/v1/workspace-settings');
    expect(fetch.calls[1].init.method).toBe('PATCH');
    expect(fetch.header(1, 'If-Match')).toBe('"4"');
    expect(JSON.parse(String(fetch.calls[1].init.body))).toEqual({ aiAssistEnabled: true });
  });
});
