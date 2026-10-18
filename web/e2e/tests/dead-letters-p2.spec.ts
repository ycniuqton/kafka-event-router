import { expect, test, type Page } from '@playwright/test';

/**
 * DL-127: bulk retry as a background job (progress bar + alerts-menu notification) and restore of a discarded event,
 * against the real API. Needs E2E_DLQ_ROUTE (default `dev-dead-letters-orders`) with more than 500 unresolved dead
 * letters for the job prompt (produce failing events to `dev-dead-letters-in`); otherwise the job part is skipped.
 */
const DeadLettersP2E2E = {
  routeName: process.env.E2E_DLQ_ROUTE ?? 'dev-dead-letters-orders',
  routes: '/api/v1/routes?limit=200',
  syncCap: 500,
  jobDoneText: /Background retry finished: \d+ events/,
  testTimeoutMs: 180_000,
  jobTimeoutMs: 150_000,
} as const;

async function devLogin(page: Page): Promise<void> {
  await page.goto('/login');
  const button = page.getByRole('button', { name: 'Dev login' });
  await button.waitFor({ timeout: 45_000 });
  await button.click();
  await page.waitForURL((url) => !url.pathname.startsWith('/login'));
}

async function deadLetterRoute(page: Page): Promise<{ id: string; unresolved: number } | null> {
  const list = await (await page.request.get(DeadLettersP2E2E.routes)).json();
  const route = list.items.find((r: { name: string }) => r.name === DeadLettersP2E2E.routeName);
  if (!route) return null;
  const summary = await (await page.request.get(`/api/v1/routes/${route.id}/dlq/summary`)).json();
  return { id: route.id, unresolved: summary.unresolved };
}

test.describe('dead letters P2', () => {
  test.setTimeout(DeadLettersP2E2E.testTimeoutMs);

  test('retry all runs as a background job with progress and a notification', async ({ page }) => {
    await devLogin(page);
    const route = await deadLetterRoute(page);
    test.skip(!route || route.unresolved <= DeadLettersP2E2E.syncCap, `route ${DeadLettersP2E2E.routeName} needs > ${DeadLettersP2E2E.syncCap} dead letters`);
    await page.goto(`/routes/${route!.id}?tab=dead`);
    await expect(page.getByRole('checkbox', { name: /Select event at offset/ }).first()).toBeVisible({ timeout: 30_000 });
    await page.getByRole('button', { name: 'Retry all', exact: true }).click();
    const dialog = page.getByRole('dialog');
    await expect(dialog.getByText('Retry all in the background?')).toBeVisible();
    await dialog.getByRole('button', { name: 'Start background retry' }).click();
    await expect(page.getByRole('progressbar', { name: 'Job progress' })).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText(DeadLettersP2E2E.jobDoneText).first()).toBeVisible({ timeout: DeadLettersP2E2E.jobTimeoutMs });
    await page.getByRole('button', { name: /^Alerts/ }).click();
    await expect(page.getByRole('menuitem', { name: DeadLettersP2E2E.jobDoneText })).toBeVisible();
  });

  test('discard, show resolved, restore', async ({ page }) => {
    await devLogin(page);
    const route = await deadLetterRoute(page);
    test.skip(!route || route.unresolved < 1, `route ${DeadLettersP2E2E.routeName} needs a dead letter`);
    await page.goto(`/dead-letters?route=${route!.id}`);
    const main = page.getByRole('main');
    const rows = main.getByRole('checkbox', { name: /Select event at offset/ });
    await expect(rows.first()).toBeVisible({ timeout: 30_000 });
    const label = (await rows.first().getAttribute('aria-label')) ?? '';
    await main.getByText(/· p\d+@\d+/).first().click();
    await main.getByRole('button', { name: 'Discard', exact: true }).click();
    await expect(main.getByRole('checkbox', { name: label, exact: true })).toHaveCount(0, { timeout: 15_000 });

    const showResolved = main.getByRole('checkbox', { name: 'Show resolved' });
    await showResolved.click();
    await expect(showResolved).toBeChecked();
    const discarded = main.getByRole('checkbox', { name: label, exact: true });
    await expect(discarded).toBeVisible({ timeout: 30_000 });
    await discarded.click();
    await expect(discarded).toBeChecked();
    await main.getByRole('button', { name: /^Restore \d+$/ }).click();
    await expect(page.getByText(/Event restored|Restored \d+ events/).first()).toBeVisible({ timeout: 15_000 });
    await showResolved.click();
    await expect(showResolved).not.toBeChecked();
    await expect(main.getByRole('checkbox', { name: label, exact: true })).toBeVisible({ timeout: 30_000 });
  });
});
