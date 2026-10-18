import { expect, test, type Page } from '@playwright/test';

/**
 * LP-206: demo workspace → builder → samples load with zero setup. A route on the demo sandbox
 * (`kind=demo` connection, topic `order-events`) fills the sample panel from the preview service's demo
 * source; no Kafka topic or upload is needed. Uses the real demo connection (POST /onboarding/demo,
 * monitoring#P3-BE-07) when it exists; otherwise the connection list is answered with the tunnel connection
 * flagged `kind=demo` (UI only — every preview call still goes to the real API). Routes: `dev-live-preview-p3-<ts>`.
 */
const E2E = {
  connectionName: process.env.E2E_PREVIEW_CONNECTION ?? 'staging-kafka-tunnel',
  bootstrap: process.env.E2E_PREVIEW_BOOTSTRAP ?? '127.0.0.1:29092',
  onboardingPath: '/api/v1/onboarding/demo',
  listPath: '/api/v1/connections',
  connectionsPattern: /\/api\/v1\/connections(\?.*)?$/,
  demoKind: 'demo',
  demoTopic: 'order-events',
  demoNotice: 'Demo data: synthetic order events (no Kafka read).',
  prefix: 'dev-live-preview-p3-',
  csrfCookie: 'er_csrf',
  csrfHeader: 'X-CSRF-Token',
  timeoutMs: 90_000,
} as const;

type ConnectionItem = { id: string; name: string; kind?: string };

async function devLogin(page: Page): Promise<void> {
  await page.goto('/login');
  const button = page.getByRole('button', { name: 'Dev login' });
  await button.waitFor({ timeout: 45_000 });
  await button.click();
  await page.waitForURL((url) => !url.pathname.startsWith('/login'));
}

async function csrf(page: Page): Promise<Record<string, string>> {
  const cookie = (await page.context().cookies()).find((c) => c.name === E2E.csrfCookie);
  return { [E2E.csrfHeader]: cookie?.value ?? '' };
}

async function listConnections(page: Page): Promise<ConnectionItem[]> {
  return ((await (await page.request.get(E2E.listPath)).json()) as { items: ConnectionItem[] }).items;
}

/** The demo sandbox connection: the real one when the onboarding endpoint provides it, else the tunnel connection shown as demo. */
async function demoSandbox(page: Page): Promise<ConnectionItem> {
  await page.request.post(E2E.onboardingPath, { headers: await csrf(page) });
  const items = await listConnections(page);
  const demo = items.find((c) => c.kind === E2E.demoKind);
  if (demo) return demo;
  const tunnel = items.find((c) => c.name.startsWith(E2E.connectionName)) ?? (await createTunnel(page));
  await page.route(E2E.connectionsPattern, async (route) => {
    const res = await route.fetch();
    const body = (await res.json()) as { items: ConnectionItem[] };
    const items = body.items.map((c) => (c.id === tunnel.id ? { ...c, kind: E2E.demoKind } : c));
    await route.fulfill({ response: res, json: { ...body, items } });
  });
  return tunnel;
}

async function createTunnel(page: Page): Promise<ConnectionItem> {
  const data = { name: E2E.connectionName, envTag: 'dev', bootstrapServers: [E2E.bootstrap], security: { mode: 'plaintext' } };
  const res = await page.request.post(E2E.listPath, { data, headers: await csrf(page) });
  expect(res.ok(), await res.text()).toBeTruthy();
  return res.json();
}

/** A draft route reading the demo order topic, keeping only paid orders. */
async function createDemoRoute(page: Page, connectionId: string): Promise<string> {
  const name = `${E2E.prefix}${Date.now()}`;
  const draft = {
    apiVersion: 'router/v1',
    name,
    source: { connectionId, topics: [E2E.demoTopic], format: { type: 'json' }, startFrom: { mode: 'latest' } },
    branches: [
      {
        id: 'main',
        filter: { type: 'and', children: [{ type: 'cond', field: 'value.eventType', op: 'eq', value: 'order.paid' }] },
        destination: { connectionId, topic: `${E2E.prefix}out`, key: { mode: 'keep' }, partitioning: 'by_key', headers: { mode: 'keep_all' }, format: { type: 'json' } },
      },
    ],
    errorPolicy: { onError: 'skip' },
  };
  const res = await page.request.post('/api/v1/routes', { data: { name, draft }, headers: await csrf(page) });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).id;
}

test.describe('live preview P3 — demo data', () => {
  test.setTimeout(E2E.timeoutMs);

  test('demo sandbox route loads synthetic samples with zero setup', async ({ page }) => {
    await devLogin(page);
    const demo = await demoSandbox(page);
    const id = await createDemoRoute(page, demo.id);
    const load = page.waitForRequest((r) => r.url().endsWith('/api/v1/preview/samples') && r.postDataJSON()?.mode === E2E.demoKind);
    await page.goto(`/routes/${id}/edit?step=filter`);
    expect((await load).postDataJSON()).toMatchObject({ mode: 'demo', topics: [E2E.demoTopic], count: 50 });
    const summary = page.getByTestId('sample-summary');
    await expect(summary).toContainText(/50 loaded · \d+ pass · \d+ filtered · 0 errors/, { timeout: 30_000 });
    await expect(page.getByTestId('sample-panel').getByText(E2E.demoNotice)).toBeVisible();
    await expect(page.getByTestId('sample-panel').getByText('ord_000050').first()).toBeVisible();
  });
});
