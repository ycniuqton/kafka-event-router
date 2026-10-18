import { expect, test, type Page } from '@playwright/test';

/**
 * DL-71: Dead letters against the real API — filter by route, select → detail, Retry (fails again → report),
 * Discard (row leaves), Export downloads JSON, Open in Studio lands on the transform step, route tab.
 * Needs the route E2E_DLQ_ROUTE (default `dev-dead-letters-orders`) deployed with a failing JS step and ≥ 2 dead letters.
 */
const DeadLettersE2E = {
  routeName: process.env.E2E_DLQ_ROUTE ?? 'dev-dead-letters-orders',
  routes: '/api/v1/routes?limit=200',
  minRows: 2,
  exportName: /^dlq-.*\.json$/,
  studioUrl: /\/routes\/[^/]+\/edit\?(?=.*step=transform)(?=.*session=)/,
  sampleText: /ord_b?\d+/,
  testTimeoutMs: 90_000,
} as const;

async function devLogin(page: Page): Promise<void> {
  await page.goto('/login');
  const button = page.getByRole('button', { name: 'Dev login' });
  await button.waitFor({ timeout: 45_000 });
  await button.click();
  await page.waitForURL((url) => !url.pathname.startsWith('/login'));
}

async function routeWithDeadLetters(page: Page): Promise<{ id: string; unresolved: number } | null> {
  const list = await (await page.request.get(DeadLettersE2E.routes)).json();
  const route = list.items.find((r: { name: string }) => r.name === DeadLettersE2E.routeName);
  if (!route) return null;
  const summary = await (await page.request.get(`/api/v1/routes/${route.id}/dlq/summary`)).json();
  return { id: route.id, unresolved: summary.unresolved };
}

test.describe('dead letters', () => {
  test.setTimeout(DeadLettersE2E.testTimeoutMs);

  test('browse, retry, discard, export and open in studio', async ({ page }) => {
    await devLogin(page);
    const route = await routeWithDeadLetters(page);
    test.skip(!route || route.unresolved < DeadLettersE2E.minRows, `route ${DeadLettersE2E.routeName} needs ≥ ${DeadLettersE2E.minRows} dead letters`);
    await page.goto(`/dead-letters?route=${route!.id}`);
    const main = page.getByRole('main');
    const rows = main.getByRole('checkbox', { name: /Select event at offset/ });
    await expect(rows.first()).toBeVisible({ timeout: 30_000 });
    await expect(main.getByText(new RegExp(`${DeadLettersE2E.routeName} · Step \\d+ · `)).first()).toBeVisible();
    const before = await rows.count();

    await main.getByText(/^p\d+@\d+$|· p\d+@\d+/).first().click();
    await expect(main.getByText('Original event', { exact: true })).toBeVisible();
    await expect(main.getByText('Loading the original event…')).toBeHidden({ timeout: 15_000 });

    await main.getByRole('button', { name: 'Retry', exact: true }).click();
    const report = page.getByRole('dialog');
    await expect(report.getByText('Retry report')).toBeVisible({ timeout: 30_000 });
    await expect(report.getByText(/1 failed/)).toBeVisible();
    await report.getByRole('button', { name: 'Close' }).last().click();
    await expect(rows).toHaveCount(before);

    const download = page.waitForEvent('download');
    await main.getByRole('button', { name: 'Export JSON' }).last().click();
    expect((await download).suggestedFilename()).toMatch(DeadLettersE2E.exportName);

    await main.getByRole('button', { name: 'Discard', exact: true }).click();
    await expect(rows).toHaveCount(before - 1, { timeout: 15_000 });

    await page.goto(`/routes/${route!.id}?tab=dead`);
    await expect(page.getByText(/Dead letters · \d+/)).toBeVisible();
    await expect(page.getByRole('checkbox', { name: /Select event at offset/ }).first()).toBeVisible({ timeout: 30_000 });
    await page.getByRole('button', { name: 'Open in Transform Studio' }).click();
    await page.waitForURL(DeadLettersE2E.studioUrl, { timeout: 30_000 });
    await expect(page.getByText(DeadLettersE2E.sampleText).first()).toBeAttached({ timeout: 30_000 });
  });
});
