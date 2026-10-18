import { expect, type Browser } from '@playwright/test';
import { AccApi, Http } from '../lib/Api';
import { DexOidcLogin } from '../lib/Login';
import { Converge, Paths } from '../lib/Platform';
import { Profile, SecretsFile, TargetProfile } from '../lib/Target';
import type { Json } from '../lib/RouteDraft';
import { ApproverKeys, DemoAlert, DemoClusters, DemoGate, DemoPartitions, DemoRoutes, DemoTopics, type DemoClusterKey, type DemoRoute } from './DemoCatalog';

/** Approval API (access-control). */
const Approvals = { path: (id: string) => `/api/v1/approvals/${id}`, pending: 'pending_approval' } as const;

/** Idempotent find-or-create of every demo- resource on staging (never deletes anything). */
export class DemoSeeder {
  readonly conn = {} as Record<DemoClusterKey, string>;
  readonly routeIds: Record<string, string> = {};

  constructor(readonly api: AccApi) {}

  /** Connections kafka-a / kafka-b / kafka-b-prod; each is tested. */
  async connections(): Promise<void> {
    const list = (await this.api.get(`${Paths.connections}?limit=200`)).body.items as { id: string; name: string }[];
    for (const key of Object.keys(DemoClusters) as DemoClusterKey[]) {
      const spec = DemoClusters[key];
      this.conn[key] = list.find((c) => c.name === spec.connection)?.id ?? (await this.createConnection(key));
      const test = await this.api.post(Paths.connectionTest(this.conn[key]), {});
      expect(test.ok, `${spec.connection} test: ${test.describe()}`).toBeTruthy();
      console.log(`connection ${spec.connection} (${spec.bootstrap}, ${spec.envTag}) test: ${test.text.slice(0, 300)}`);
    }
  }

  private async createConnection(key: DemoClusterKey): Promise<string> {
    const s = DemoClusters[key];
    const res = await this.api.post<{ id: string }>(Paths.connections, {
      name: s.connection, description: s.description, envTag: s.envTag, bootstrapServers: [s.bootstrap], security: { mode: 'plaintext' }, allowTopicCreate: true, owner: { teamId: await this.api.teamId() },
    });
    expect(res.status, res.describe()).toBe(201);
    return res.body.id;
  }

  /** Demo topics, 3 partitions (409 = exists). */
  async topics(): Promise<void> {
    for (const key of Object.keys(DemoTopics) as DemoClusterKey[]) {
      for (const name of DemoTopics[key]) {
        const res = await this.api.post(Paths.connectionTopics(this.conn[key]), { name, partitions: DemoPartitions });
        expect([201, 409], `${name}: ${res.describe()}`).toContain(res.status);
      }
    }
  }

  /** Creates (or finds) every demo route, deploys it when not running and waits for running. */
  async routes(): Promise<void> {
    for (const r of DemoRoutes) {
      const id = await this.ensureRoute(r.name, r.description, r.build(this.conn));
      await this.ensureRunning(id);
    }
  }

  /** Saved Studio test cases (created once by name), then all of them are run against the route's draft. */
  async testCases(): Promise<void> {
    for (const r of DemoRoutes) await this.ensureCases(r);
  }

  private async ensureCases(r: DemoRoute): Promise<void> {
    const id = this.routeIds[r.name];
    const existing: { id: string; name: string; version: number }[] = (await this.api.get(`${Paths.route(id)}/test-cases`)).body?.items ?? [];
    for (const c of r.cases) {
      const found = existing.find((x) => x.name === c.name);
      const res = found
        ? await this.api.patch(`${Paths.route(id)}/test-cases/${found.id}`, { expectedOutcome: c.expectedOutcome, expectedOutput: c.expectedOutput ?? null }, `"${found.version}"`)
        : await this.api.post(`${Paths.route(id)}/test-cases`, c);
      expect(res.ok, `${r.name}/${c.name}: ${res.describe()}`).toBeTruthy();
    }
    const run = await this.api.post(`${Paths.route(id)}/test-cases/run`, { config: r.build(this.conn) });
    expect(run.status, run.describe()).toBe(200);
    console.log(`test cases ${r.name}: passed=${run.body.passed} failed=${run.body.failed}`);
    expect(run.body.failed, JSON.stringify(run.body.results, null, 1)).toBe(0);
  }

  async ensureRoute(name: string, description: string, draft: Json): Promise<string> {
    const found = ((await this.api.get(`${Paths.routes}?q=${name}&limit=50`)).body?.items ?? []).find((x: { name: string }) => x.name === name);
    if (found) return (this.routeIds[name] = found.id);
    const res = await this.api.post<{ id: string }>(Paths.routes, { name, description, teamId: await this.api.teamId(), draft });
    expect(res.status, res.describe()).toBe(201);
    return (this.routeIds[name] = res.body.id);
  }

  async ensureRunning(id: string): Promise<void> {
    const route = (await this.api.get(Paths.route(id))).body;
    if (route.observedState !== 'running') {
      const dep = await this.api.withEtag(Http.Post, Paths.routeAction(id, 'deploy'), { changeNote: 'demo: initial version' }, Paths.route(id));
      expect(dep.status, dep.describe()).toBe(202);
    }
    await expect.poll(async () => (await this.api.get(Paths.route(id))).body?.observedState, { timeout: Converge.routeRunningMs * 2, intervals: [2_000] }).toBe('running');
  }

  /** One alert rule on a demo route (created once by name). */
  async alertRule(): Promise<void> {
    const rules = (await this.api.get(Paths.alertRules)).body?.items ?? [];
    if (rules.some((r: { name: string }) => r.name === DemoAlert.name)) return;
    const { route, ...rule } = DemoAlert;
    const res = await this.api.post(Paths.alertRules, { ...rule, routeId: this.routeIds[route], channels: [] });
    expect(res.status, res.describe()).toBe(201);
  }
}

/** The second person of 4-eyes deploys: signs the approver in through Dex and gives it the approver role. */
export class DemoApprover {
  constructor(
    private readonly admin: AccApi,
    private readonly secrets = new SecretsFile(),
  ) {}

  async signIn(browser: Browser): Promise<AccApi> {
    const creds = { email: this.secrets.get(ApproverKeys.email), password: this.secrets.get(ApproverKeys.password) };
    expect(creds.email && creds.password, `${ApproverKeys.email}/${ApproverKeys.password} missing in the credentials file`).toBeTruthy();
    const profile = new TargetProfile(Profile.target, Profile.baseURL, Profile.login, Profile.platformBootstrap, Profile.runnerBootstrap, creds);
    const context = await browser.newContext({ baseURL: Profile.baseURL });
    await new DexOidcLogin(profile).signIn(context);
    const api = new AccApi(context);
    await this.grantRole((await api.me()).id);
    return new AccApi(context);
  }

  private async grantRole(userId: string): Promise<void> {
    const teamId = await this.admin.teamId();
    const user = (await this.admin.get(Paths.user(userId))).body;
    const binding = (user.teams ?? []).find((t: { teamId: string }) => t.teamId === teamId);
    if (binding?.role === ApproverKeys.role) return;
    const put = await this.admin.put(`/api/v1/role-bindings/${userId}/${teamId}`, { role: ApproverKeys.role }, binding ? `"${binding.rowVersion}"` : undefined);
    expect(put.ok, put.describe()).toBeTruthy();
  }
}

/** demo-approval-gate: v1 approved by the approver and running; a v2 change left pending approval. */
export class DemoGateFlow {
  constructor(
    private readonly seeder: DemoSeeder,
    private readonly approver: AccApi,
  ) {}

  async run(): Promise<void> {
    const api = this.seeder.api;
    const id = await this.seeder.ensureRoute(DemoGate.name, DemoGate.description, DemoGate.build(this.seeder.conn, DemoGate.approvedThreshold));
    const route = (await api.get(Paths.route(id))).body;
    if (!route.activeVersion) await this.approveFirstVersion(id);
    await expect.poll(async () => (await api.get(Paths.route(id))).body?.observedState, { timeout: Converge.routeRunningMs * 2, intervals: [2_000] }).toBe('running');
    if (!(await this.pending(id))) await this.requestChange(id);
  }

  private async approveFirstVersion(id: string): Promise<void> {
    const dep = await this.deploy(id, 'demo: v1 (amount >= 100) - approved by staging-approver');
    const detail = await this.approver.get(Approvals.path(dep.id));
    const decided = await this.approver.post(`${Approvals.path(dep.id)}/approve`, { note: 'demo: 4-eyes approval' }, detail.etag || `"${detail.body.approval.rowVersion}"`);
    expect(decided.status, decided.describe()).toBe(200);
  }

  private async requestChange(id: string): Promise<void> {
    const api = this.seeder.api;
    const saved = await api.withEtag(Http.Patch, Paths.route(id), { draft: DemoGate.build(this.seeder.conn, DemoGate.pendingThreshold) });
    expect(saved.ok, saved.describe()).toBeTruthy();
    await this.deploy(id, 'demo: raise threshold to 200 - waiting for a second person');
  }

  private async deploy(id: string, changeNote: string): Promise<any> {
    const res = await this.seeder.api.withEtag(Http.Post, Paths.routeAction(id, 'deploy'), { changeNote }, Paths.route(id));
    expect(res.status, res.describe()).toBe(202);
    expect(res.body.deployment.status).toBe(Approvals.pending);
    return res.body.deployment;
  }

  private async pending(id: string): Promise<boolean> {
    const list = (await this.seeder.api.get(`/api/v1/approvals?status=${Approvals.pending}`)).body?.items ?? [];
    return list.some((a: { routeId?: string; route?: { id: string } }) => (a.routeId ?? a.route?.id) === id);
  }
}
