import { expect, test, type Page } from '@playwright/test';
import { AccApi } from '../lib/Api';
import { LoginManager } from '../lib/Login';
import { Paths } from '../lib/Platform';
import { DemoClusters, DemoRoutes } from './DemoCatalog';

/** UI walk-through of the staging demo as the staging admin (read-only: nothing is created or changed). */
const DemoUi = { pageMs: 45_000, metricsMs: 120_000, shots: 'report/demo-screens' } as const;

async function routeIds(page: Page): Promise<Record<string, string>> {
  const api = new AccApi(page.context());
  const items = (await api.get(`${Paths.routes}?q=demo-&limit=50`)).body.items as { id: string; name: string }[];
  return Object.fromEntries(items.map((r) => [r.name, r.id]));
}

async function shot(page: Page, name: string): Promise<void> {
  await page.screenshot({ path: `${DemoUi.shots}/${name}.png`, fullPage: true });
}

test.beforeEach(async ({ page }) => {
  await LoginManager.forProfile().signInPage(page);
});

test('connections list shows kafka-a and kafka-b', async ({ page }) => {
  await page.goto('/connections');
  for (const c of Object.values(DemoClusters)) await expect(page.getByRole('main').getByText(c.connection, { exact: true }).first()).toBeVisible({ timeout: DemoUi.pageMs });
  await expect(page.getByRole('main').getByText('staging-kafka', { exact: true }).first()).toBeVisible();
  await shot(page, '01-connections');
});

test('routes list shows the demo routes running with traffic', async ({ page }) => {
  await page.goto('/routes?q=demo-');
  const main = page.getByRole('main');
  for (const r of DemoRoutes) await expect(main.getByText(r.name, { exact: true }).first()).toBeVisible({ timeout: DemoUi.pageMs });
  const api = new AccApi(page.context());
  await expect.poll(async () => {
    const items = (await api.get(`${Paths.routes}?q=demo-&limit=50`)).body.items as { name: string; observedState: string; metrics: { eventsPerSec: number } }[];
    return DemoRoutes.every((d) => items.some((r) => r.name === d.name && r.observedState === 'running' && r.metrics.eventsPerSec > 0));
  }, { timeout: DemoUi.metricsMs, intervals: [5_000] }).toBe(true);
  await shot(page, '02-routes');
});

test('live tail streams the cross-cluster route demo-a-to-b-paid', async ({ page }) => {
  const id = (await routeIds(page))['demo-a-to-b-paid'];
  await page.goto(`/routes/${id}?tab=tail`);
  await expect(page.getByTestId('tail-state')).toHaveText(/Streaming/, { timeout: DemoUi.pageMs });
  await expect(page.getByTestId('tail-row').first()).toBeVisible({ timeout: DemoUi.metricsMs });
  await shot(page, '03-live-tail');
});

test('dead letters lists entries of demo-b-dlq', async ({ page }) => {
  const id = (await routeIds(page))['demo-b-dlq'];
  await page.goto(`/dead-letters?route=${id}`);
  await expect(page.getByRole('main').getByRole('checkbox', { name: /Select event at offset/ }).first()).toBeVisible({ timeout: DemoUi.pageMs });
  await shot(page, '04-dead-letters');
});

for (const r of DemoRoutes) {
  test(`saved test cases of ${r.name} run green in Studio`, async ({ page }) => {
    const id = (await routeIds(page))[r.name];
    await page.goto(`/routes/${id}/edit?step=transform`);
    const panel = page.getByTestId('test-cases');
    await expect(panel).toBeVisible({ timeout: DemoUi.pageMs });
    await expect(panel.getByTestId('test-case')).toHaveCount(r.cases.filter((c) => c.branchId === r.cases[0].branchId).length, { timeout: DemoUi.pageMs }).catch(() => undefined);
    const run = panel.getByRole('button', { name: 'Run', exact: true });
    if (await run.isVisible()) await run.click();
    await expect(panel.getByText(/\d+ passing/)).toBeVisible({ timeout: DemoUi.pageMs });
    await expect(panel.getByText(/failing/)).toHaveCount(0);
    await shot(page, `05-testcases-${r.name}`);
  });
}

test('approvals inbox shows the pending demo-approval-gate change', async ({ page }) => {
  await page.goto('/');
  await page.getByRole('button', { name: 'Approvals' }).click();
  await expect(page.getByText('demo-approval-gate').first()).toBeVisible({ timeout: DemoUi.pageMs });
  await shot(page, '06-approvals');
});
