import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

/**
 * DL-209 (+ DL-204/DL-206 in the UI): the demo workspace ships seeded dead letters on its `order-events` route.
 * Each run creates a fresh route on the `kind=demo` connection (in-memory cluster of the API process, nothing reaches
 * the shared Kafka) whose JS step fails on orders without items → Dead letters lists the 12 seeded failures →
 * Open in Studio carries the failing event → fix (draft saved + deployed through the API, the code editor is not
 * scripted) → Retry succeeds. An exactly_once twin shows the at-least-once warning; the audit preset lists the actions.
 */
const P3 = {
  routes: '/api/v1/routes',
  connections: '/api/v1/connections?limit=200',
  onboarding: '/api/v1/onboarding/demo',
  me: '/api/v1/auth/me',
  demoKind: 'demo',
  prefix: 'dev-dead-letters-demo-',
  seeded: 12,
  failingStepOffset: 4200,
  studioUrl: /\/routes\/[^/]+\/edit\?(?=.*step=transform)(?=.*session=)/,
  csrfCookie: 'er_csrf',
  testTimeoutMs: 120_000,
} as const;

const brokenCode = 'function transform(event, ctx) {\n  const total = event.value.items.reduce((s, i) => s + i.qty * i.price, 0);\n  return { ...event, value: { orderId: event.value.orderId, total } };\n}';
const fixedCode = brokenCode.replace('event.value.items.reduce', '(event.value.items || []).reduce');

type Draft = Record<string, unknown>;

/** API calls of the fixture (CSRF double-submit + If-Match from the route ETag). */
class DemoApi {
  constructor(private readonly page: Page) {}

  private get request(): APIRequestContext {
    return this.page.request;
  }

  private async headers(etag?: string): Promise<Record<string, string>> {
    const cookies = await this.page.context().cookies();
    const csrf = cookies.find((c) => c.name === P3.csrfCookie)?.value ?? '';
    return { 'X-CSRF-Token': csrf, ...(etag ? { 'If-Match': etag } : {}) };
  }

  async demoConnection(): Promise<{ id: string } | undefined> {
    await this.request.post(P3.onboarding, { headers: await this.headers() });
    const list = await (await this.request.get(P3.connections)).json();
    return list.items.find((c: { kind?: string }) => c.kind === P3.demoKind);
  }

  async teamId(): Promise<string> {
    const me = await (await this.request.get(P3.me)).json();
    return Object.keys(me.access.teams)[0];
  }

  draft(name: string, connectionId: string, code: string, exactlyOnce: boolean): Draft {
    return {
      apiVersion: 'router/v1', name,
      source: { connectionId, topics: ['order-events'], format: { type: 'json' }, consumerGroup: `router.${name}`, startFrom: { mode: 'latest' } },
      branches: [{ id: 'main', transform: { steps: [{ id: 'total', kind: 'js', code }] }, destination: { connectionId, topic: 'order-totals', format: { type: 'json' }, createTopic: { partitions: 3 } } }],
      errorPolicy: { onError: 'dlq' },
      ...(exactlyOnce ? { delivery: { mode: 'exactly_once' } } : {}),
    };
  }

  async createDeployed(name: string, connectionId: string, code: string, exactlyOnce = false): Promise<string> {
    const body = { name, teamId: await this.teamId(), description: 'e2e fixture (dead-letters P3)', tags: ['dev'], draft: this.draft(name, connectionId, code, exactlyOnce) };
    const created = await this.request.post(P3.routes, { data: body, headers: await this.headers() });
    expect(created.ok(), await created.text()).toBe(true);
    const id = (await created.json()).id;
    await this.deploy(id);
    return id;
  }

  async etag(id: string): Promise<string> {
    return (await this.request.get(`${P3.routes}/${id}`)).headers()['etag'];
  }

  async deploy(id: string): Promise<void> {
    const res = await this.request.post(`${P3.routes}/${id}/deploy`, { data: {}, headers: await this.headers(await this.etag(id)) });
    expect(res.ok(), await res.text()).toBe(true);
  }

  async fix(id: string, name: string, connectionId: string): Promise<void> {
    const res = await this.request.patch(`${P3.routes}/${id}`, { data: { draft: this.draft(name, connectionId, fixedCode, false) }, headers: await this.headers(await this.etag(id)) });
    expect(res.ok(), await res.text()).toBe(true);
    await this.deploy(id);
  }

  async archive(id: string): Promise<void> {
    await this.request.post(`${P3.routes}/${id}/pause`, { data: {}, headers: await this.headers(await this.etag(id)) });
    await this.request.delete(`${P3.routes}/${id}`, { headers: await this.headers(await this.etag(id)) });
  }
}

async function devLogin(page: Page): Promise<void> {
  await page.goto('/login');
  const button = page.getByRole('button', { name: 'Dev login' });
  await button.waitFor({ timeout: 45_000 });
  await button.click();
  await page.waitForURL((url) => !url.pathname.startsWith('/login'));
}

test.describe('dead letters P3', () => {
  test.setTimeout(P3.testTimeoutMs);

  test('demo workspace: seeded dead letters → Open in Studio → fix → Retry succeeds', async ({ page }) => {
    await devLogin(page);
    const api = new DemoApi(page);
    const demo = await api.demoConnection();
    test.skip(!demo, 'needs a kind=demo connection (POST /onboarding/demo or connections CreateDemo)');
    const name = `${P3.prefix}${Date.now()}`;
    const id = await api.createDeployed(name, demo!.id, brokenCode);
    try {
      await page.goto(`/dead-letters?route=${id}`);
      const main = page.getByRole('main');
      const rows = main.getByRole('checkbox', { name: /Select event at offset/ });
      await expect(rows).toHaveCount(P3.seeded, { timeout: 30_000 });
      await main.getByText(new RegExp(`p0@${P3.failingStepOffset}`)).first().click();
      await expect(main.getByText(/Transform error · Step 1 · JavaScript/)).toBeVisible();
      await expect(main.getByText("Cannot read properties of null (reading 'reduce')").first()).toBeVisible();

      await main.getByRole('button', { name: 'Open in Transform Studio' }).click();
      await page.waitForURL(P3.studioUrl, { timeout: 30_000 });
      await expect(page.getByText(/ord_\d+/).first()).toBeAttached({ timeout: 30_000 });

      await api.fix(id, name, demo!.id);
      await page.goto(`/dead-letters?route=${id}`);
      await expect(rows).toHaveCount(P3.seeded, { timeout: 30_000 });
      await main.getByText(new RegExp(`p0@${P3.failingStepOffset}`)).first().click();
      await main.getByRole('button', { name: 'Retry', exact: true }).click();
      await expect(page.getByText(/Retried 1 event · sent/)).toBeVisible({ timeout: 30_000 });
      await expect(rows).toHaveCount(P3.seeded - 1, { timeout: 15_000 });
    } finally {
      await api.archive(id);
    }
  });

  test('exactly-once route warns that retries are at-least-once; audit preset lists dead-letter actions', async ({ page }) => {
    await devLogin(page);
    const api = new DemoApi(page);
    const demo = await api.demoConnection();
    test.skip(!demo, 'needs a kind=demo connection');
    const id = await api.createDeployed(`${P3.prefix}eo-${Date.now()}`, demo!.id, fixedCode, true);
    try {
      await page.goto(`/dead-letters?route=${id}`);
      const main = page.getByRole('main');
      await expect(main.getByRole('checkbox', { name: /Select event at offset/ })).toHaveCount(P3.seeded, { timeout: 30_000 });
      await main.getByText(new RegExp(`p0@${P3.failingStepOffset}`)).first().click();
      await expect(main.getByTestId('dlq-at-least-once')).toBeVisible();
      await main.getByRole('button', { name: 'Retry', exact: true }).click();
      const dialog = page.getByRole('dialog');
      await expect(dialog.getByText('Retry report')).toBeVisible({ timeout: 30_000 });
      await expect(dialog.getByTestId('dlq-at-least-once')).toBeVisible();
      await dialog.getByRole('button', { name: 'Close' }).last().click();

      await main.getByRole('button', { name: 'Audit trail →' }).click();
      await page.waitForURL(/\/audit\?preset=dlq/);
      await expect(page.getByRole('button', { name: 'Dead-letter actions', pressed: true })).toBeVisible();
      await expect(page.locator('[title="dlq.retried"]').first()).toHaveText('Retried dead letters', { timeout: 15_000 });
    } finally {
      await api.archive(id);
    }
  });
});
