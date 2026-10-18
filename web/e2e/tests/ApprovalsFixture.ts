import type { APIRequestContext } from '@playwright/test';
import { AccessApi, AccessSession, Csrf } from './AccessFixture';

/** Names of everything the approval scenarios create (shared Kafka: `dev-access-control-` prefix). */
export class ApprovalNames {
  readonly stamp = Date.now();
  readonly prefix = `dev-access-control-e2e-${this.stamp}`;
  readonly connection = `${this.prefix}-prod`;
  readonly route = `${this.prefix}-route`;
  readonly inTopic = `${this.prefix}-in`;
  readonly outTopic = `${this.prefix}-out`;

  email(role: string): string {
    return `${this.prefix}-${role}@local.dev`;
  }
}

/** Bootstrap of connections created by tests (the dev tunnel). */
export const E2EBootstrap = process.env.E2E_BOOTSTRAP ?? '127.0.0.1:29092';

/** A JSON API session with CSRF and If-Match handling (one browser context's cookies). */
export class ApiSession extends AccessSession {
  constructor(private readonly ctx: APIRequestContext) {
    super(ctx);
  }

  async call<T>(method: string, path: string, body?: unknown, ifMatch?: string | null): Promise<{ status: number; body: T; etag: string }> {
    const state = await this.ctx.storageState();
    const headers: Record<string, string> = { [Csrf.header]: state.cookies.find((c) => c.name === Csrf.cookie)?.value ?? '' };
    if (ifMatch) headers['If-Match'] = ifMatch;
    const res = await this.ctx.fetch(path, { method, headers, data: body === undefined ? undefined : body });
    const text = await res.text();
    return { status: res.status(), body: (text ? JSON.parse(text) : null) as T, etag: res.headers()['etag'] ?? '' };
  }

  /** Clears workspace admin and sets the role in team for userId (this session must be an admin). */
  async setRole(userId: string, teamId: string, role: string): Promise<void> {
    const user = await this.call<{ teams: { teamId: string; rowVersion: number }[] }>('GET', AccessApi.user(userId));
    await this.call('PATCH', AccessApi.user(userId), { workspaceAdmin: false }, user.etag);
    const fresh = await this.call<{ teams: { teamId: string; rowVersion: number }[] }>('GET', AccessApi.user(userId));
    const version = fresh.body.teams.find((t) => t.teamId === teamId)?.rowVersion;
    const put = await this.call('PUT', AccessApi.binding(userId, teamId), { role }, `"${version}"`);
    if (put.status !== 200) throw new Error(`set role: ${put.status}`);
  }
}

/** The route draft the scenario deploys: one branch to the prod connection. */
export class ScenarioDraft {
  static of(names: ApprovalNames, connectionId: string, start: string): unknown {
    return {
      apiVersion: 'router/v1', name: names.route,
      source: { format: { type: 'json' }, topics: [names.inTopic], startFrom: { mode: start }, connectionId, consumerGroup: `router.${names.route}` },
      branches: [{ id: 'main', destination: { key: { mode: 'keep' }, topic: names.outTopic, format: { type: 'same_as_source' }, headers: { mode: 'keep_all' }, connectionId, partitioning: 'by_key' } }],
      errorPolicy: { onError: 'dlq' },
    };
  }
}
