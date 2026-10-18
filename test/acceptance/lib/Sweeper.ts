import { chromium, type FullConfig } from '@playwright/test';
import { AccApi, Http } from './Api';
import { BrowserEnv } from './BrowserEnv';
import { AccKafka } from './Kafka';
import { AccPrefix } from './Names';
import { Paths } from './Platform';

/** Kafka name prefixes of everything the suite may leave behind on the shared cluster. */
export const LeftoverPrefixes = {
  topics: [AccPrefix, `router.dlq.${AccPrefix}`, `router.test.${AccPrefix}`],
  groups: [`router.${AccPrefix}`, 'acc-verify-'],
} as const;

/** User statuses the users API reports. */
enum UserStatus {
  Disabled = 'disabled',
}

/** Best-effort removal of acc- resources a crashed or interrupted run left behind (re-runnable suite). */
export class Sweeper {
  constructor(
    private readonly api: AccApi,
    private readonly kafka: AccKafka = new AccKafka(),
  ) {}

  static async run(config: FullConfig): Promise<void> {
    const browser = await chromium.launch(BrowserEnv.launchOptions());
    try {
      const context = await browser.newContext({ baseURL: config.projects[0].use.baseURL });
      await new Sweeper(await AccApi.signIn(context)).sweep();
    } finally {
      await browser.close();
    }
  }

  async sweep(): Promise<void> {
    await this.routes();
    await this.connections();
    await this.teams();
    await this.tokens();
    await this.users();
    await this.alertRules();
    await this.topics();
  }

  private async routes(): Promise<void> {
    const list = await this.api.get(`${Paths.routes}?q=${AccPrefix}&status=draft,running,paused,error&limit=200`);
    for (const r of (list.body?.items ?? []).filter((x: { name: string }) => x.name.startsWith(AccPrefix))) {
      await this.api.withEtag(Http.Post, Paths.routeAction(r.id, 'pause'), {}, Paths.route(r.id)).catch(() => undefined);
      await this.api.withEtag(Http.Delete, Paths.route(r.id)).catch(() => undefined);
    }
  }

  private async connections(): Promise<void> {
    const list = await this.api.get(`${Paths.connections}?q=${AccPrefix}&limit=200`);
    for (const c of (list.body?.items ?? []).filter((x: { name: string }) => x.name.startsWith(AccPrefix))) await this.api.delete(Paths.connection(c.id)).catch(() => undefined);
  }

  private async teams(): Promise<void> {
    const list = await this.api.get('/api/v1/teams');
    for (const t of (list.body?.items ?? []).filter((x: { name: string }) => x.name.startsWith(AccPrefix))) await this.api.withEtag(Http.Delete, `/api/v1/teams/${t.id}`).catch(() => undefined);
  }

  private async tokens(): Promise<void> {
    const list = await this.api.get('/api/v1/auth/tokens');
    for (const t of (list.body?.items ?? []).filter((x: { name: string }) => x.name.startsWith(AccPrefix))) await this.api.delete(`/api/v1/auth/tokens/${t.id}`).catch(() => undefined);
  }

  /** Disables acc- users (invited or signed in by tests); users cannot be deleted. */
  private async users(): Promise<void> {
    const list = await this.api.get(`${Paths.users}?q=${AccPrefix}&limit=200`);
    const active = (list.body?.items ?? []).filter((u: { email: string; status: string }) => u.email.startsWith(AccPrefix) && u.status !== UserStatus.Disabled);
    for (const u of active) await this.api.withEtag(Http.Patch, Paths.user(u.id), { disabled: true }).catch(() => undefined);
  }

  private async alertRules(): Promise<void> {
    const list = await this.api.get(Paths.alertRules);
    for (const r of (list.body?.items ?? []).filter((x: { name: string }) => x.name.startsWith(AccPrefix))) await this.api.delete(`${Paths.alertRules}/${r.id}`).catch(() => undefined);
  }

  private async topics(): Promise<void> {
    const topics: string[] = [];
    for (const prefix of LeftoverPrefixes.topics) topics.push(...(await this.kafka.topicsWithPrefix(prefix).catch(() => [])));
    await this.kafka.deleteTopics(topics).catch(() => undefined);
    const groups = await this.kafka.groupsWithPrefixes([...LeftoverPrefixes.groups]).catch(() => []);
    await this.kafka.deleteGroups(groups).catch(() => undefined);
  }
}
