import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

/**
 * P2-T-07 against the real stack (VITE_DATA_SOURCE=http, dev login): create an alert rule in the route Settings tab,
 * see it fire through WS /events (toast + bell) on a running route, and open a route from the dashboard flow map.
 * Needs the deployed route `dev-monitoring-alerts` (override with E2E_MONITORING_ROUTE); skipped otherwise.
 */
const MonitoringP2E2E = {
  routeName: process.env.E2E_MONITORING_ROUTE ?? 'dev-monitoring-alerts',
  routes: '/api/v1/routes?limit=200',
  rules: '/api/v1/alert-rules',
  ruleName: `dev-monitoring-e2e-${Date.now()}`,
  webhook: 'https://127.0.0.1:18999/e2e-hook',
  fireTimeoutMs: 120_000,
  testTimeoutMs: 180_000,
} as const;

type RouteRow = { id: string; name: string; status: string };

async function devLogin(page: Page): Promise<void> {
  await page.goto('/login');
  const button = page.getByRole('button', { name: 'Dev login' });
  await button.waitFor({ timeout: 45_000 });
  await button.click();
  await page.waitForURL((url) => !url.pathname.startsWith('/login'));
}

/** The CSRF header the API expects on cookie-authenticated writes. */
async function csrf(page: Page): Promise<Record<string, string>> {
  const cookie = (await page.context().cookies()).find((c) => c.name === 'er_csrf');
  return { 'X-CSRF-Token': cookie?.value ?? '' };
}

async function findRoute(page: Page): Promise<RouteRow | undefined> {
  const list = await (await page.request.get(MonitoringP2E2E.routes)).json();
  return (list.items as RouteRow[]).find((r) => r.name === MonitoringP2E2E.routeName);
}

/** Pauses or resumes the route through the routes API (If-Match from GET). */
async function setRunning(page: Page, request: APIRequestContext, id: string, running: boolean): Promise<void> {
  const current = await request.get(`/api/v1/routes/${id}`);
  const etag = current.headers()['etag'] ?? '';
  const body = await current.json();
  if ((body.status === 'running') === running) return;
  const res = await request.post(`/api/v1/routes/${id}/${running ? 'resume' : 'pause'}`, { headers: { ...(await csrf(page)), 'If-Match': etag } });
  expect(res.ok()).toBeTruthy();
}

async function deleteE2ERules(page: Page, routeId: string): Promise<void> {
  const rules = await (await page.request.get(`${MonitoringP2E2E.rules}?routeId=${routeId}`)).json();
  for (const r of rules.items as { id: string; name: string }[]) {
    if (r.name.startsWith('dev-monitoring-e2e-')) await page.request.delete(`${MonitoringP2E2E.rules}/${r.id}`, { headers: await csrf(page) });
  }
}

test.describe('monitoring P2', () => {
  test.setTimeout(MonitoringP2E2E.testTimeoutMs);

  test('alert rule from the Settings tab fires through push into the bell', async ({ page }) => {
    await devLogin(page);
    const route = await findRoute(page);
    test.skip(!route, `route ${MonitoringP2E2E.routeName} is not deployed in this workspace`);
    await deleteE2ERules(page, route!.id);
    await page.goto(`/routes/${route!.id}?tab=settings`);
    const panel = page.locator('[aria-label="Alert rules"]');
    await panel.getByRole('button', { name: '+ Add rule' }).click();
    const form = page.locator('[aria-label="Alert rule"]');
    await form.getByLabel('Name').fill(MonitoringP2E2E.ruleName);
    await form.getByLabel('When').selectOption('throughput_zero');
    await form.getByLabel('For (minutes)').fill('1');
    await form.getByLabel('Add channel').selectOption('webhook');
    await form.getByLabel('Webhook webhook URL').fill('ftp://nope');
    await form.getByRole('button', { name: 'Save rule' }).click();
    await expect(form.getByText('Enter the full https:// URL of the incoming webhook.')).toBeVisible();
    await form.getByLabel('Webhook webhook URL').fill(MonitoringP2E2E.webhook);
    const created = page.waitForResponse((r) => r.url().endsWith(MonitoringP2E2E.rules) && r.request().method() === 'POST');
    await form.getByRole('button', { name: 'Save rule' }).click();
    expect((await created).status()).toBe(201);
    await expect(panel.getByText(MonitoringP2E2E.ruleName)).toBeVisible();
    await expect(panel.getByText('Throughput is 0 for 1 min · Webhook').first()).toBeVisible();

    await setRunning(page, page.request, route!.id, true);
    try {
      await expect(page.getByText(`Alert firing: ${MonitoringP2E2E.routeName} · ${MonitoringP2E2E.ruleName}`)).toBeVisible({ timeout: MonitoringP2E2E.fireTimeoutMs });
      await page.getByRole('button', { name: /^Alerts/ }).click();
      const row = page.getByRole('menuitem', { name: new RegExp(`${MonitoringP2E2E.routeName} · ${MonitoringP2E2E.ruleName}`) }).first();
      await expect(row).toBeVisible();
      await row.click();
      await expect(page).toHaveURL(new RegExp(`/routes/${route!.id}`));
    } finally {
      await setRunning(page, page.request, route!.id, false);
      await deleteE2ERules(page, route!.id);
    }
  });

  test('flow map is drawn from the API and a line opens its route', async ({ page }) => {
    await devLogin(page);
    const flow = page.waitForResponse((r) => r.url().includes('/api/v1/flow-map') && r.ok());
    await page.goto('/');
    const map = await (await flow).json();
    test.skip(map.edges.length === 0, 'no deployed route to draw');
    const edge = page.locator('.react-flow__edge').first();
    await expect(edge).toBeVisible();
    await edge.locator('path').last().click({ force: true });
    await expect(page).toHaveURL(/\/routes\/[0-9a-f-]+/);
  });
});
