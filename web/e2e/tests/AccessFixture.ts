import type { APIRequestContext } from '@playwright/test';

/** API paths the access scenarios drive (through the vite proxy). */
export const AccessApi = {
  devLogin: '/auth/dev/login',
  me: '/api/v1/auth/me',
  user: (id: string) => `/api/v1/users/${id}`,
  binding: (userId: string, teamId: string) => `/api/v1/role-bindings/${userId}/${teamId}`,
} as const;

/** Header and cookie names of the double-submit CSRF token. */
export const Csrf = { header: 'X-CSRF-Token', cookie: 'er_csrf' } as const;

/** A signed-in API session (one browser context's cookies). */
export class AccessSession {
  constructor(private readonly request: APIRequestContext) {}

  /** Dev login as `email` (dev mode seeds the user as admin of the default team). */
  async login(email?: string): Promise<void> {
    const res = await this.request.post(AccessApi.devLogin, { data: email ? { email } : {} });
    if (!res.ok()) throw new Error(`dev login failed: ${res.status()}`);
  }

  async me(): Promise<{ id: string; teams: { id: string }[] }> {
    return (await this.request.get(AccessApi.me)).json();
  }

  private async csrf(): Promise<string> {
    const state = await this.request.storageState();
    return state.cookies.find((c) => c.name === Csrf.cookie)?.value ?? '';
  }

  /** Clears workspace admin and sets the team role (admin-only calls, If-Match from the current row versions). */
  async demote(userId: string, teamId: string, role: string): Promise<void> {
    const headers = { [Csrf.header]: await this.csrf() };
    const user = await this.request.get(AccessApi.user(userId));
    const etag = user.headers()['etag'];
    const patched = await this.request.patch(AccessApi.user(userId), { data: { workspaceAdmin: false }, headers: { ...headers, 'If-Match': etag } });
    if (!patched.ok()) throw new Error(`patch user: ${patched.status()}`);
    const version = (await patched.json()).teams.find((t: { teamId: string }) => t.teamId === teamId).rowVersion;
    const put = await this.request.put(AccessApi.binding(userId, teamId), { data: { role }, headers: { ...headers, 'If-Match': `"${version}"` } });
    if (!put.ok()) throw new Error(`put binding: ${put.status()}`);
  }
}
