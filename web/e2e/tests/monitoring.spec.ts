import { expect, test, type Page } from '@playwright/test';

/**
 * M-T-08: the dashboard renders health cards, needs-attention and recent activity from the real API,
 * and the route Overview switches range through `GET /routes/{id}/metrics` (VITE_DATA_SOURCE=http, dev login).
 */
const MonitoringE2E = {
  summary: '/api/v1/dashboard/summary',
  attention: '/api/v1/dashboard/attention',
  activity: '/api/v1/activity',
  routes: '/api/v1/routes?limit=1',
  metricsPath: /\/api\/v1\/routes\/[^/]+\/metrics\?range=/,
  cards: ['Routes', 'Events forwarded · last hour', 'Total consumer lag', 'Dead-letter events · 24 h'],
  ranges: ['15m', '24h', '7d'],
  testTimeoutMs: 60_000,
} as const;

async function devLogin(page: Page): Promise<void> {
  await page.goto('/login');
  const button = page.getByRole('button', { name: 'Dev login' });
  await button.waitFor({ timeout: 45_000 });
  await button.click();
  await page.waitForURL((url) => !url.pathname.startsWith('/login'));
}

test.describe('monitoring', () => {
  test.setTimeout(MonitoringE2E.testTimeoutMs);

  test('dashboard renders cards, attention and activity from the API', async ({ page }) => {
    await devLogin(page);
    const summary = page.waitForResponse((r) => r.url().includes(MonitoringE2E.summary) && r.ok());
    const attention = page.waitForResponse((r) => r.url().includes(MonitoringE2E.attention) && r.ok());
    const activity = page.waitForResponse((r) => r.url().includes(MonitoringE2E.activity) && r.ok());
    await page.goto('/');
    const body = await (await summary).json();
    await attention;
    const feed = await (await activity).json();

    const main = page.getByRole('main');
    for (const label of MonitoringE2E.cards) await expect(main.getByText(label, { exact: true })).toBeVisible();
    await expect(main.getByText(`${body.routes.running} running`)).toBeVisible();
    await expect(main.getByText(/Updated \d+ s ago · live/)).toBeVisible();
    await expect(main.getByText('Needs attention', { exact: true })).toBeVisible();
    await expect(main.getByText('Recent activity', { exact: true })).toBeVisible();
    if (feed.items.length > 0) await expect(main.getByText(feed.items[0].routeName).first()).toBeVisible();
  });

  test('route overview switches range against the metrics API', async ({ page }) => {
    await devLogin(page);
    const list = await (await page.request.get(MonitoringE2E.routes)).json();
    test.skip(list.items.length === 0, 'no route in this workspace');
    const route = list.items[0];
    await page.goto(`/routes/${route.id}?tab=overview`);
    const group = page.getByRole('radiogroup', { name: 'Time range' });
    await expect(group).toBeVisible();
    for (const range of MonitoringE2E.ranges) {
      const response = page.waitForResponse((r) => MonitoringE2E.metricsPath.test(r.url()) && r.url().endsWith(`range=${range}`));
      await group.getByRole('radio', { name: range }).click();
      expect((await response).status()).toBe(200);
      await expect(group.getByRole('radio', { name: range })).toHaveAttribute('aria-checked', 'true');
      await expect(page.getByText(/read$/).first()).toBeVisible();
    }
  });
});
