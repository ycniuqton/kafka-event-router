import { expect, test, type Page } from '@playwright/test';

/**
 * Live preview P2 on the real stack: masked values on a sensitive connection (lock marks), per-session
 * format override, and "Also write to test topic" in the Test dialog. Needs the `dev-live-preview-p2-orders`
 * topic (6 JSON orders); every connection and route it creates is named `dev-live-preview-p2-e2e-<ts>`.
 */
const E2E = {
  bootstrap: process.env.E2E_PREVIEW_BOOTSTRAP ?? '127.0.0.1:29092',
  topic: process.env.E2E_PREVIEW_P2_TOPIC ?? 'dev-live-preview-p2-orders',
  prefix: 'dev-live-preview-p2-e2e-',
  csrfCookie: 'er_csrf',
  csrfHeader: 'X-CSRF-Token',
  timeoutMs: 90_000,
} as const;

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

/** Creates a fresh connection (optionally sensitive) and a draft route with two branches reading the P2 topic. */
async function createRoute(page: Page, sensitive: boolean): Promise<string> {
  const name = `${E2E.prefix}${Date.now()}`;
  const data = { name, envTag: 'dev', bootstrapServers: [E2E.bootstrap], security: { mode: 'plaintext' }, sensitive, allowTopicCreate: true };
  const conn = await page.request.post('/api/v1/connections', { data, headers: await csrf(page) });
  expect(conn.ok(), await conn.text()).toBeTruthy();
  const connId = (await conn.json()).id;
  const dest = { connectionId: connId, topic: `${E2E.prefix}out`, key: { mode: 'keep' }, partitioning: 'by_key', headers: { mode: 'keep_all' }, format: { type: 'json' } };
  const draft = {
    apiVersion: 'router/v1',
    name,
    source: { connectionId: connId, topics: [E2E.topic], format: { type: 'json' }, startFrom: { mode: 'latest' } },
    branches: [{ id: 'main', filter: { type: 'cond', field: 'value.type', op: 'eq', value: 'sale' }, destination: dest }],
    errorPolicy: { onError: 'skip' },
  };
  const res = await page.request.post('/api/v1/routes', { data: { name, draft }, headers: await csrf(page) });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).id;
}

test.describe('live preview P2', () => {
  test.setTimeout(E2E.timeoutMs);

  test('sensitive connection shows masked values with lock marks', async ({ page }) => {
    await devLogin(page);
    const id = await createRoute(page, true);
    await page.goto(`/routes/${id}/edit?step=filter`);
    const panel = page.getByTestId('sample-panel');
    await expect(page.getByTestId('sample-summary')).toContainText('6 loaded', { timeout: 30_000 });
    await expect(panel.getByRole('note')).toBeVisible();
    await expect(panel.getByTestId('masked-mark').first()).toBeVisible();
    await expect(panel).not.toContainText('shop.dev');
    await panel.getByRole('radio', { name: 'Raw' }).click().catch(() => panel.getByText('Raw', { exact: true }).click());
    await expect(panel.getByText('masked').first()).toBeVisible();
  });

  test('format override and test topic write', async ({ page }) => {
    await devLogin(page);
    const id = await createRoute(page, false);
    await page.goto(`/routes/${id}/edit?step=filter`);
    const panel = page.getByTestId('sample-panel');
    const summary = page.getByTestId('sample-summary');
    await expect(summary).toContainText('6 loaded · 4 pass · 2 filtered', { timeout: 30_000 });
    await expect(panel).toContainText('shop.dev');

    await panel.getByTestId('sample-format').selectOption('string');
    await expect(panel.getByTestId('sample-format')).toHaveValue('string', { timeout: 15_000 });
    await panel.getByTestId('sample-format').selectOption('json');
    await expect(summary).toContainText('4 pass · 2 filtered', { timeout: 15_000 });

    await page.getByRole('button', { name: 'Test', exact: true }).click();
    await expect(page.getByTestId('test-run-report')).toBeVisible({ timeout: 30_000 });
    await page.getByTestId('write-test-topic').click();
    await expect(page.getByTestId('test-topic')).toContainText(/Wrote 4 outputs to router\.test\.dev-live-preview-p2-e2e-\d+\.\d{14}/, { timeout: 30_000 });
  });
});
