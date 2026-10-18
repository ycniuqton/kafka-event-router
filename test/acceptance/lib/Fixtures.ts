import { test as base, expect, type Browser, type BrowserContext } from '@playwright/test';
import { AccApi, Http } from './Api';
import { LoginManager } from './Login';
import { AccNames } from './Names';
import { Workspace } from './Platform';
import { Profile } from './Target';

/** Team roles (access-control). */
export enum Role {
  Viewer = 'viewer',
  Editor = 'editor',
  Approver = 'approver',
  Admin = 'admin',
}

/** A second signed-in user with a team role (local target only: dev login with any email). */
export class UserFactory {
  private readonly contexts: BrowserContext[] = [];
  private readonly userIds: string[] = [];

  constructor(
    private readonly browser: Browser,
    private readonly admin: AccApi,
    private readonly names: AccNames,
  ) {}

  /** Signs in a new user and sets its role in the admin's team (workspace admin cleared). */
  async as(role: Role): Promise<AccApi> {
    base.skip(!Profile.canImpersonate, `target ${Profile.target} has a single login user; multi-role scenario needs ACC_TARGET=local`);
    const context = await this.browser.newContext();
    this.contexts.push(context);
    const api = await AccApi.signIn(context, this.names.email(role));
    const me = await api.me();
    this.userIds.push(me.id);
    await this.setRole(me.id, await this.admin.teamId(), role);
    return new AccApi(context);
  }

  /** Closes the extra sessions and disables the throw-away users. */
  async close(): Promise<void> {
    for (const c of this.contexts) await c.close().catch(() => undefined);
    for (const id of this.userIds) await this.admin.withEtag(Http.Patch, `/api/v1/users/${id}`, { disabled: true }).catch(() => undefined);
  }

  private async setRole(userId: string, teamId: string, role: Role): Promise<void> {
    const patched = await this.admin.withEtag(Http.Patch, `/api/v1/users/${userId}`, { workspaceAdmin: false });
    expect(patched.ok, patched.describe()).toBeTruthy();
    const fresh = await this.admin.get(`/api/v1/users/${userId}`);
    const version = fresh.body.teams.find((t: { teamId: string }) => t.teamId === teamId)?.rowVersion;
    const put = await this.admin.put(`/api/v1/role-bindings/${userId}/${teamId}`, { role }, version === undefined ? undefined : `"${version}"`);
    expect(put.ok, put.describe()).toBeTruthy();
  }
}

/** Per-test fixtures: the admin API session, a workspace with automatic cleanup, extra users. */
type AccFixtures = { api: AccApi; names: AccNames; ws: Workspace; users: UserFactory; signedInPage: void };

export const test = base.extend<AccFixtures>({
  names: async ({}, use, info) => {
    await use(new AccNames(info.titlePath[0].replace(/\.spec\.ts$/, '').replace(/[^a-z0-9]+/gi, '').slice(0, 10).toLowerCase()));
  },
  api: async ({ context }, use) => {
    await use(await AccApi.signIn(context));
  },
  ws: async ({ api, names }, use) => {
    const ws = new Workspace(api, names);
    await use(ws);
    await ws.resources.cleanup();
  },
  users: async ({ browser, api, names }, use) => {
    const users = new UserFactory(browser, api, names);
    await use(users);
    await users.close();
  },
  signedInPage: async ({ page }, use) => {
    await LoginManager.forProfile().signInPage(page);
    await use();
  },
});

export { expect };
