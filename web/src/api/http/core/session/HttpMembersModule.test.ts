import { describe, expect, it } from 'vitest';
import { Role, RoleKey, UserStatus } from '../../../../domain';
import { HttpContext } from '../HttpContext';
import { TestFetch } from '../TestFetch.test-util';
import { HttpMembersService } from './HttpMembersModule';

const user = {
  id: 'u1', email: 'hoa@x.io', displayName: 'Hoa L.', workspaceAdmin: false, status: UserStatus.Active, rowVersion: 3,
  teams: [{ teamId: 't1', teamName: 'Logistics', role: RoleKey.Editor, rowVersion: 2 }],
};

describe('HttpMembersService', () => {
  it('lists members from /users (names and team names, no mock fallback)', async () => {
    const fetch = new TestFetch().json(200, { items: [user], next_cursor: null });
    const svc = new HttpMembersService(new HttpContext({ apiBaseUrl: '/api/v1', authBaseUrl: '', fetch: fetch.fn }));
    expect(await svc.list()).toEqual([{ name: 'Hoa L.', role: Role.Editor, team: 'Logistics' }]);
    expect(fetch.calls[0].url).toContain('/api/v1/users');
  });
});
