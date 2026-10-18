import { expect } from '@playwright/test';
import { AccApi, Http, type ApiResponse } from './Api';
import { AccKafka } from './Kafka';
import { AccNames } from './Names';
import { RouteDraft, type BranchSpec, type Json } from './RouteDraft';
import { Profile } from './Target';

/** API paths of the platform the suite drives. */
export const Paths = {
  connections: '/api/v1/connections',
  connection: (id: string) => `/api/v1/connections/${id}`,
  connectionTest: (id: string) => `/api/v1/connections/${id}/test`,
  connectionTopics: (id: string) => `/api/v1/connections/${id}/topics`,
  routes: '/api/v1/routes',
  route: (id: string) => `/api/v1/routes/${id}`,
  routeAction: (id: string, action: string) => `/api/v1/routes/${id}/${action}`,
  preview: { samples: '/api/v1/preview/samples', run: '/api/v1/preview/run', testRuns: '/api/v1/preview/test-runs', infer: '/api/v1/preview/infer-mapping' },
  dlq: (id: string) => `/api/v1/routes/${id}/dlq`,
  alertRules: '/api/v1/alert-rules',
  auditLog: '/api/v1/audit-log',
  teams: '/api/v1/teams',
  team: (id: string) => `/api/v1/teams/${id}`,
  users: '/api/v1/users',
  user: (id: string) => `/api/v1/users/${id}`,
} as const;

/**
 * Long-lived acc- teams reused by every run. A team that ever owned a route can never be deleted (routes are only
 * archived and still count as owned objects), so scenarios that need a route on a second team share one fixture team.
 */
export class FixtureTeams {
  static readonly routeOwner = 'acc-fixture-route-owner';

  constructor(private readonly api: AccApi) {}

  /** Id of the team with this name, created on first use. */
  async ensure(name: string): Promise<string> {
    const list = await this.api.get<{ items: { id: string; name: string }[] }>(Paths.teams);
    const found = (list.body?.items ?? []).find((t) => t.name === name);
    if (found) return found.id;
    const created = await this.api.post<{ id: string }>(Paths.teams, { name });
    expect(created.status, created.describe()).toBe(201);
    return created.body.id;
  }
}

/** Timing of runtime convergence (workers reconcile via NOTIFY, 15 s resync). */
export const Converge = { routeRunningMs: 60_000, pollMs: 1_000 } as const;

/** Records everything a test creates and removes it again (reverse order), even when the test fails. */
export class ResourceRegistry {
  private readonly routes: string[] = [];
  private readonly connections: string[] = [];
  private readonly topics: string[] = [];
  private readonly groups: string[] = [];
  private readonly extra: (() => Promise<unknown>)[] = [];
  private readonly releases: (() => Promise<unknown>)[] = [];

  constructor(
    private readonly api: AccApi,
    private readonly kafka: AccKafka,
  ) {}

  route(id: string): string {
    this.routes.push(id);
    return id;
  }

  connection(id: string): string {
    this.connections.push(id);
    return id;
  }

  topic(...names: string[]): void {
    this.topics.push(...names);
  }

  group(...names: string[]): void {
    this.groups.push(...names);
  }

  onCleanup(fn: () => Promise<unknown>): void {
    this.extra.push(fn);
  }

  /** Runs fn before routes are archived, to drop references an archived route would keep (e.g. library steps). */
  onRelease(fn: () => Promise<unknown>): void {
    this.releases.push(fn);
  }

  /** Releases references, archives routes, then removes the objects routes used (teams, library objects, rules). */
  async cleanup(): Promise<void> {
    for (const fn of this.releases) await fn().catch(() => undefined);
    for (const id of this.routes) await this.archiveRoute(id).catch(() => undefined);
    for (const fn of this.extra.reverse()) await fn().catch(() => undefined);
    await this.kafka.deleteTopics(this.topics).catch(() => undefined);
    await this.kafka.deleteGroups(this.groups).catch(() => undefined);
    for (const id of this.connections) await this.api.delete(Paths.connection(id)).catch(() => undefined);
  }

  private async archiveRoute(id: string): Promise<void> {
    await this.api.withEtag(Http.Post, Paths.routeAction(id, 'pause'), {}, Paths.route(id));
    await this.api.withEtag(Http.Delete, Paths.route(id));
  }
}

/** A connection + topics + route set-up used by most runtime scenarios. */
export class Workspace {
  readonly kafka = new AccKafka();
  readonly resources: ResourceRegistry;

  constructor(
    readonly api: AccApi,
    readonly names: AccNames,
  ) {
    this.resources = new ResourceRegistry(api, this.kafka);
  }

  /** Creates a plaintext connection on the platform's bootstrap (topic creation allowed). */
  async connection(overrides: Json = {}, name = this.names.connection): Promise<string> {
    const res = await this.api.post<{ id: string }>(Paths.connections, {
      name, envTag: 'dev', bootstrapServers: [Profile.platformBootstrap], security: { mode: 'plaintext' }, allowTopicCreate: true, owner: { teamId: await this.api.teamId() }, ...overrides,
    });
    expect(res.status, res.describe()).toBe(201);
    return this.resources.connection(res.body.id);
  }

  async topics(connectionId: string, names: string[], partitions = 1): Promise<void> {
    for (const name of names) {
      const res = await this.api.post(Paths.connectionTopics(connectionId), { name, partitions });
      expect([201, 409], res.describe()).toContain(res.status);
      this.resources.topic(name);
    }
  }

  /** Connection + in/out topics; returns the connection id. */
  async pipe(partitions = 1): Promise<string> {
    const conn = await this.connection();
    await this.topics(conn, [this.names.inTopic, this.names.outTopic], partitions);
    this.resources.topic(this.names.dlqTopic());
    this.resources.group(this.names.group());
    return conn;
  }

  draft(connectionId: string, branches: BranchSpec[] = [{ topic: this.names.outTopic }], name = this.names.route): RouteDraft {
    const d = new RouteDraft(name, connectionId, [this.names.inTopic]);
    for (const b of branches) d.branch(b);
    return d;
  }

  async createRoute(draft: RouteDraft | Json, extra: Json = {}): Promise<string> {
    const doc = draft instanceof RouteDraft ? draft.build() : draft;
    const res = await this.api.post<{ id: string }>(Paths.routes, { name: doc.name, teamId: await this.api.teamId(), draft: doc, ...extra });
    expect(res.status, res.describe()).toBe(201);
    return this.resources.route(res.body.id);
  }

  async deploy(routeId: string, changeNote = 'acceptance'): Promise<ApiResponse> {
    return this.api.withEtag(Http.Post, Paths.routeAction(routeId, 'deploy'), { changeNote }, Paths.route(routeId));
  }

  async saveDraft(routeId: string, draft: RouteDraft | Json): Promise<ApiResponse> {
    const doc = draft instanceof RouteDraft ? draft.build() : draft;
    return this.api.withEtag(Http.Patch, Paths.route(routeId), { draft: doc });
  }

  /** Create + deploy + wait until a runner reports running. */
  async runningRoute(draft: RouteDraft): Promise<string> {
    const id = await this.createRoute(draft);
    const dep = await this.deploy(id);
    expect(dep.status, dep.describe()).toBe(202);
    await this.waitObserved(id, 'running');
    return id;
  }

  async waitObserved(routeId: string, state: string, timeoutMs: number = Converge.routeRunningMs): Promise<void> {
    await expect
      .poll(async () => (await this.api.get(Paths.route(routeId))).body?.observedState, { timeout: timeoutMs, intervals: [Converge.pollMs] })
      .toBe(state);
  }

  /** Waits until every replica runs version `no` (the deployment itself turns active after the auto-rollback window). */
  async waitRunningVersion(routeId: string, no: number, timeoutMs: number = Converge.routeRunningMs): Promise<void> {
    await expect
      .poll(async () => {
        const replicas = (await this.api.get(`${Paths.route(routeId)}/runtime`)).body?.replicas ?? [];
        return replicas.length > 0 && replicas.every((r: { versionNo: number; state: string }) => r.versionNo === no && r.state === 'running');
      }, { timeout: timeoutMs, intervals: [Converge.pollMs] })
      .toBe(true);
  }

  async route(routeId: string): Promise<any> {
    return (await this.api.get(Paths.route(routeId))).body;
  }

  async pause(routeId: string): Promise<ApiResponse> {
    return this.api.withEtag(Http.Post, Paths.routeAction(routeId, 'pause'), {}, Paths.route(routeId));
  }

  async resume(routeId: string): Promise<ApiResponse> {
    return this.api.withEtag(Http.Post, Paths.routeAction(routeId, 'resume'), {}, Paths.route(routeId));
  }

  /** Upload-mode sample session; returns the session id. */
  async uploadSession(events: Json[]): Promise<string> {
    const res = await this.api.post(Paths.preview.samples, { mode: 'upload', events });
    expect(res.ok, res.describe()).toBeTruthy();
    return res.body.session.id;
  }

  async previewRun(sessionId: string, config: Json): Promise<ApiResponse> {
    return this.api.post(Paths.preview.run, { sessionId, config });
  }
}
