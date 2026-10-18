import { expect, test, type Page } from '@playwright/test';

/**
 * P3-T-06 against the real stack (VITE_DATA_SOURCE=http, dev login): live tail (records, text filter, show-only, pause),
 * demo data endpoints, and the replay confirmation dialog. Needs the deployed route `dev-monitoring-tail` reading the
 * demo connection's `order-events` (override with E2E_MONITORING_TAIL_ROUTE); skipped otherwise. The route is resumed
 * for the test and paused again afterwards.
 */
const MonitoringP3E2E = {
  routeName: process.env.E2E_MONITORING_TAIL_ROUTE ?? 'dev-monitoring-tail',
  routes: '/api/v1/routes?limit=200',
  onboarding: '/api/v1/onboarding',
  demo: '/api/v1/onboarding/demo',
  recordTimeoutMs: 30_000,
  testTimeoutMs: 120_000,
  noMatch: 'no-such-value-in-any-event-xyz',
} as const;

type RouteRow = { id: string; name: string; status: string };

async function devLogin(page: Page): Promise<void> {
  await page.goto('/login');
  const button = page.getByRole('button', { name: 'Dev login' });
  await button.waitFor({ timeout: 45_000 });
  await button.click();
  await page.waitForURL((url) => !url.pathname.startsWith('/login'));
}

async function csrf(page: Page): Promise<Record<string, string>> {
  const cookie = (await page.context().cookies()).find((c) => c.name === 'er_csrf');
  return { 'X-CSRF-Token': cookie?.value ?? '' };
}

async function findRoute(page: Page): Promise<RouteRow | undefined> {
  const list = await (await page.request.get(MonitoringP3E2E.routes)).json();
  return (list.items as RouteRow[]).find((r) => r.name === MonitoringP3E2E.routeName);
}

/** Pauses or resumes the route through the routes API (If-Match from GET). */
async function setRunning(page: Page, id: string, running: boolean): Promise<void> {
  const current = await page.request.get(`/api/v1/routes/${id}`);
  const body = await current.json();
  if ((body.status === 'running') === running) return;
  const headers = { ...(await csrf(page)), 'If-Match': current.headers()['etag'] ?? '' };
  const res = await page.request.post(`/api/v1/routes/${id}/${running ? 'resume' : 'pause'}`, { headers });
  expect(res.ok(), await res.text()).toBeTruthy();
}

test.describe('monitoring P3', () => {
  test.setTimeout(MonitoringP3E2E.testTimeoutMs);

  test('live tail streams records; filter, show-only and pause work', async ({ page }) => {
    await devLogin(page);
    const route = await findRoute(page);
    test.skip(!route, `needs the deployed route ${MonitoringP3E2E.routeName}`);
    await setRunning(page, route!.id, true);
    try {
      await page.goto(`/routes/${route!.id}?tab=tail`);
      const state = page.getByTestId('tail-state');
      await expect(state).toHaveText(/Streaming/, { timeout: MonitoringP3E2E.recordTimeoutMs });
      const rows = page.getByTestId('tail-row');
      await expect(rows.first()).toBeVisible({ timeout: MonitoringP3E2E.recordTimeoutMs });
      await rows.first().click();
      await expect(page.getByText(/^Input · order-events/)).toBeVisible();

      const filter = page.getByLabel('Filter the tail');
      await filter.fill(MonitoringP3E2E.noMatch);
      await expect(rows).toHaveCount(0);
      await expect(page.getByText('No events match. Clear the filter or wait for new events.')).toBeVisible();
      await filter.fill('');
      await expect(rows.first()).toBeVisible({ timeout: MonitoringP3E2E.recordTimeoutMs });

      await page.getByRole('radio', { name: 'Only errors' }).click();
      await expect(rows).toHaveCount(0);
      await page.getByRole('radio', { name: 'All' }).click();

      await page.getByRole('button', { name: 'Pause stream' }).click();
      await expect(state).toHaveText('Stream paused');
      await page.getByRole('button', { name: 'Resume stream' }).click();
      await expect(state).toHaveText(/Streaming/);
    } finally {
      await setRunning(page, route!.id, false);
    }
  });

  test('demo data: seeding is idempotent and the checklist reports the demo connection', async ({ page }) => {
    await devLogin(page);
    const created = await page.request.post(MonitoringP3E2E.demo, { headers: await csrf(page) });
    expect([201, 409]).toContain(created.status());
    const again = await page.request.post(MonitoringP3E2E.demo, { headers: await csrf(page) });
    expect(again.status()).toBe(409);
    const onboarding = await (await page.request.get(MonitoringP3E2E.onboarding)).json();
    expect(onboarding.demoAvailable).toBe(true);
    expect(onboarding.demo.topic).toBe('order-events');
    await page.goto('/connections');
    await expect(page.getByText('demo-orders').first()).toBeVisible();
  });

  test('replay asks for a past time, the duplicate acknowledgement and the typed route name', async ({ page }) => {
    await devLogin(page);
    const route = await findRoute(page);
    test.skip(!route, `needs the deployed route ${MonitoringP3E2E.routeName}`);
    await page.goto(`/routes/${route!.id}`);
    await page.getByRole('button', { name: 'Replay', exact: true }).click();
    const dialog = page.getByRole('dialog');
    const confirm = dialog.getByRole('button', { name: 'Replay', exact: true });
    await expect(confirm).toBeDisabled();
    await dialog.getByLabel('Start from (UTC)').fill('2099-01-01T00:00');
    await expect(dialog.getByText('The replay start is in the future. Pick a time in the past.')).toBeVisible();
    await dialog.getByLabel('Start from (UTC)').fill(new Date(Date.now() - 600_000).toISOString().slice(0, 16));
    await dialog.getByRole('checkbox').check();
    await dialog.getByLabel('Route name').fill('wrong-name');
    await expect(confirm).toBeDisabled();
    await dialog.getByLabel('Route name').fill(route!.name);
    await expect(confirm).toBeEnabled();
    await confirm.click();
    await expect(page.getByText(/Replay started|already running|not running/).first()).toBeVisible({ timeout: 15_000 });
  });
});
