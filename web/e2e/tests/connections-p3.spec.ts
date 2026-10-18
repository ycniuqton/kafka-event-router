import { expect, test, type Page } from '@playwright/test';

/**
 * CN-203/CN-204: a `kind=demo` connection shows the Demo badge (list + topic browser), has no Edit, passes the full
 * checklist and peeks live sample order events. Needs a demo connection: POST /onboarding/demo (monitoring#P3-BE-07)
 * when available, otherwise one named `demo-orders` seeded through connections `Service.CreateDemo`; skipped if absent.
 */
const E2E = {
  demoName: process.env.E2E_DEMO_CONNECTION ?? 'demo-orders',
  demoKind: 'demo',
  topic: 'order-events',
  onboardingPath: '/api/v1/onboarding/demo',
  listPath: '/api/v1/connections?limit=200',
  testTimeoutMs: 60_000,
} as const;

type ConnectionItem = { id: string; name: string; kind: string };

async function devLogin(page: Page): Promise<void> {
  await page.goto('/login');
  const button = page.getByRole('button', { name: 'Dev login' });
  await button.waitFor({ timeout: 45_000 });
  await button.click();
  await page.waitForURL((url) => !url.pathname.startsWith('/login'));
}

/** Finds the demo connection, asking the onboarding endpoint to create it first when that endpoint exists. */
async function demoConnection(page: Page): Promise<ConnectionItem | undefined> {
  const csrf = (await page.context().cookies()).find((c) => c.name.endsWith('csrf'))?.value ?? '';
  await page.request.post(E2E.onboardingPath, { headers: { 'X-CSRF-Token': csrf } });
  const res = await page.request.get(E2E.listPath);
  const body = (await res.json()) as { items: ConnectionItem[] };
  return body.items.find((c) => c.kind === E2E.demoKind && c.name === E2E.demoName) ?? body.items.find((c) => c.kind === E2E.demoKind);
}

test.describe('connections P3 — demo data', () => {
  test.setTimeout(E2E.testTimeoutMs);

  test('demo badge, no edit, full checklist and live order events', async ({ page }) => {
    await devLogin(page);
    const demo = await demoConnection(page);
    test.skip(!demo, 'no demo connection (needs monitoring#P3-BE-07 or a seeded demo-orders connection)');
    await page.goto('/connections');
    const row = page.getByRole('row').filter({ hasText: demo!.name });
    await expect(row.getByTestId('demo-badge')).toBeVisible();
    await expect(row.getByRole('button', { name: 'Edit' })).toHaveCount(0);

    await row.getByRole('button', { name: 'Test' }).click();
    const drawer = page.getByRole('dialog');
    await expect(drawer.getByText('All checks passed.')).toBeVisible({ timeout: 30_000 });
    await drawer.getByLabel('Read check topic').selectOption(E2E.topic);
    await drawer.getByLabel('Write check topic (optional)').selectOption(E2E.topic);
    await drawer.getByRole('button', { name: 'Test again' }).click();
    await expect(drawer.getByText('All checks passed.')).toBeVisible({ timeout: 30_000 });
    await drawer.getByRole('button', { name: 'Close' }).click();

    await row.getByRole('button', { name: 'Topics' }).click();
    const topics = page.getByRole('dialog');
    await expect(topics.getByTestId('demo-badge')).toBeVisible();
    await expect(topics.getByText(E2E.topic, { exact: true })).toBeVisible();
    await topics.getByRole('button', { name: 'Peek' }).first().click();
    await expect(topics.getByRole('button', { name: /"cust_id":"c_\d{4}"/ }).first()).toBeVisible({ timeout: 15_000 });
  });
});
