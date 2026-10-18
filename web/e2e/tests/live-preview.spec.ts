import { expect, test, type Page } from '@playwright/test';

/**
 * LP-65: load samples → edit filter → counts update (preview service) → "Why?" highlights the rows,
 * then Test. Needs the local stack and the `dev-live-preview-orders` topic (60 order events, see
 * docs/features/live-preview); every route this creates is named `dev-live-preview-e2e-<ts>`.
 */
const E2E = {
  connectionName: process.env.E2E_PREVIEW_CONNECTION ?? 'staging-kafka-tunnel',
  bootstrap: process.env.E2E_PREVIEW_BOOTSTRAP ?? '127.0.0.1:29092',
  topic: process.env.E2E_PREVIEW_TOPIC ?? 'dev-live-preview-orders',
  prefix: 'dev-live-preview-e2e-',
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

/** Returns the tunnel connection, creating it when a fresh database has none yet. */
async function ensureConnection(page: Page): Promise<{ id: string }> {
  const conns = await (await page.request.get('/api/v1/connections')).json();
  const found = conns.items.find((c: { name: string }) => c.name.startsWith(E2E.connectionName));
  if (found) return found;
  const data = { name: E2E.connectionName, envTag: 'dev', bootstrapServers: [E2E.bootstrap], security: { mode: 'plaintext' } };
  const res = await page.request.post('/api/v1/connections', { data, headers: await csrf(page) });
  expect(res.ok(), await res.text()).toBeTruthy();
  return res.json();
}

/** Creates a draft route reading the order topic with filter eventType = OrderPaid AND country = VN. */
async function createRoute(page: Page): Promise<string> {
  const conn = await ensureConnection(page);
  const name = `${E2E.prefix}${Date.now()}`;
  const cond = (field: string, value: string) => ({ type: 'cond', field, op: 'eq', value });
  const draft = {
    apiVersion: 'router/v1',
    name,
    source: { connectionId: conn.id, topics: [E2E.topic], format: { type: 'json' }, startFrom: { mode: 'latest' } },
    branches: [
      {
        id: 'main',
        filter: { type: 'and', children: [cond('value.eventType', 'OrderPaid'), cond('value.country', 'VN')] },
        transform: { steps: [{ kind: 'rename', from: 'value.cust_id', to: 'value.customerId' }] },
        destination: { connectionId: conn.id, topic: `${E2E.prefix}out`, key: { mode: 'keep' }, partitioning: 'by_key', headers: { mode: 'keep_all' }, format: { type: 'json' } },
      },
    ],
    errorPolicy: { onError: 'skip' },
  };
  const res = await page.request.post('/api/v1/routes', { data: { name, draft }, headers: await csrf(page) });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).id;
}

test.describe('live preview', () => {
  test.setTimeout(E2E.timeoutMs);

  test('load samples → edit filter → counts update → why-filtered → test', async ({ page }) => {
    await devLogin(page);
    const id = await createRoute(page);
    await page.goto(`/routes/${id}/edit?step=filter`);
    const summary = page.getByTestId('sample-summary');
    const panel = page.getByTestId('sample-panel');
    await expect(summary).toContainText('50 loaded ·', { timeout: 30_000 });
    await panel.getByLabel('Sample count').selectOption('100');
    await panel.getByRole('button', { name: 'Load', exact: true }).click();
    await expect(summary).toContainText('60 loaded · 24 pass · 36 filtered · 0 errors', { timeout: 30_000 });
    await expect(page.getByText('Matches 24 of 60 sample events')).toBeVisible();

    const firstRow = page.getByRole('combobox', { name: 'Value' }).first();
    await firstRow.selectOption('OrderCreated');
    await expect(summary).toContainText('60 loaded · 0 pass · 60 filtered', { timeout: 15_000 });
    await firstRow.selectOption('OrderPaid');
    await expect(summary).toContainText('24 pass · 36 filtered', { timeout: 15_000 });

    const filteredItem = panel.locator('[data-seq]').filter({ hasText: '→ false' }).first();
    await filteredItem.getByRole('button', { name: 'Why?' }).click();
    await expect(page.getByText(/Showing why event #\d+ was filtered out/)).toBeVisible();
    await expect(page.getByText('✕ false').first()).toBeVisible();

    await page.getByRole('button', { name: 'Test', exact: true }).click();
    await expect(page.getByTestId('test-run-report')).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText('All outputs are valid for the destination.')).toBeVisible();
  });

  test('upload works without a source connection', async ({ page }) => {
    await devLogin(page);
    await page.goto('/routes/new?template=blank');
    const panel = page.getByTestId('sample-panel');
    await panel.getByRole('radio', { name: 'Upload' }).click().catch(() => panel.getByText('Upload', { exact: true }).click());
    await panel.locator('input[type=file]').setInputFiles({
      name: 'events.ndjson',
      mimeType: 'application/json',
      buffer: Buffer.from('{"value":{"a":1}}\n{"value":{"a":2}}\n{"a":3}\n'),
    });
    await expect(page.getByTestId('sample-summary')).toContainText('3 loaded', { timeout: 15_000 });
  });
});
