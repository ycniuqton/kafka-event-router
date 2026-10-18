import { expect, test, type Page } from '@playwright/test';

/**
 * routes#m-test-e2e: connection → New route (Filter & forward) → filter row via quick filter → destination
 * → deploy → Running in the list, then Settings and History against the real API. Everything it creates is
 * named `dev-routes-e2e-<ts>` (connection, topics, route); the route is archived at the end.
 */
const E2E = {
  bootstrap: process.env.E2E_BOOTSTRAP ?? '127.0.0.1:29092',
  prefix: 'dev-routes-e2e-',
  csrfCookie: 'er_csrf',
  csrfHeader: 'X-CSRF-Token',
  timeoutMs: 120_000,
  samples: ['{"id":1,"type":"OrderPaid","amount":120}', '{"id":2,"type":"OrderCreated","amount":40}', '{"id":3,"type":"OrderPaid","amount":7}'].join('\n'),
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

/** Creates the test connection and its input/output topics through the API. */
async function createConnection(page: Page, name: string, topicPrefix: string): Promise<string> {
  const headers = await csrf(page);
  const res = await page.request.post('/api/v1/connections', {
    headers,
    data: { name, envTag: 'dev', bootstrapServers: [E2E.bootstrap], security: { mode: 'plaintext' }, allowTopicCreate: true },
  });
  expect(res.ok(), await res.text()).toBeTruthy();
  const { id } = await res.json();
  for (const topic of [`${topicPrefix}-in`, `${topicPrefix}-out`]) {
    const created = await page.request.post(`/api/v1/connections/${id}/topics`, { headers, data: { name: topic, partitions: 1 } });
    expect(created.ok(), await created.text()).toBeTruthy();
  }
  return id;
}

/** Picks the `<option>` whose label contains `text` in a select. */
async function pick(page: Page, label: string | null, text: string, index = 0): Promise<void> {
  const select = label ? page.getByLabel(label).nth(index) : page.locator('select').first();
  const value = await select.locator('option', { hasText: text }).first().getAttribute('value');
  await select.selectOption(value ?? '');
}

/** Source step: name, connection and input topic of the new route. */
async function fillSource(page: Page, name: string): Promise<void> {
  await page.getByRole('textbox', { name: 'Route name' }).fill(name);
  await pick(page, null, `${name}-conn`);
  await page.getByRole('option', { name: new RegExp(`${name}-in`) }).click();
}

/** Filter step: drop the template row, upload samples and add `type = OrderPaid` from the sample panel. */
async function quickFilter(page: Page): Promise<void> {
  await page.getByRole('button', { name: /^Filter/ }).first().click();
  await page.getByRole('button', { name: 'Remove condition' }).first().click();
  await page.getByRole('radio', { name: 'Upload' }).click();
  await page.locator('input[type="file"]').setInputFiles({ name: 'orders.ndjson', mimeType: 'application/x-ndjson', buffer: Buffer.from(E2E.samples) });
  await expect(page.getByText(/3 loaded/)).toBeVisible({ timeout: 20_000 });
  await page.getByText('"OrderPaid"').first().click();
  await expect(page.getByText(/Matches 2 of 3 sample events/)).toBeVisible({ timeout: 20_000 });
}

test.describe('routes', () => {
  test.setTimeout(E2E.timeoutMs);

  test('new route → quick filter → destination → deploy → running, settings and history', async ({ page }) => {
    const name = `${E2E.prefix}${Date.now()}`;
    await devLogin(page);
    await createConnection(page, `${name}-conn`, name);
    await page.goto('/routes/new?template=filter');
    await fillSource(page, name);
    await quickFilter(page);
    await page.getByRole('button', { name: /^Destination/ }).first().click();
    await page.locator('input[list="destination-topics"]').fill(`${name}-out`);
    await page.getByRole('button', { name: 'Save draft' }).click();
    await page.waitForURL(/\/routes\/[0-9a-f-]+\/edit/);
    await page.getByRole('button', { name: /Review & deploy/ }).click();
    await expect(page.getByText('Configuration is valid')).toBeVisible({ timeout: 20_000 });
    await page.getByRole('button', { name: 'Deploy route' }).click();
    await page.getByPlaceholder(/Change note/).fill('e2e first deploy');
    await page.getByRole('button', { name: 'Deploy', exact: true }).click();
    await page.waitForURL(/\/routes\/[0-9a-f-]+\?tab=overview/);
    const routeId = new URL(page.url()).pathname.split('/')[2];

    await page.goto(`/routes?q=${name}`);
    await expect(page.getByText(name, { exact: true })).toBeVisible();
    await expect(page.getByText('Running').first()).toBeVisible();

    await page.goto(`/routes/${routeId}?tab=settings`);
    await page.getByLabel('Tags').fill('e2e');
    await page.getByRole('button', { name: 'Save settings' }).click();
    await expect(page.getByText('Settings saved')).toBeVisible();
    await page.goto(`/routes?tag=e2e&q=${name}`);
    await expect(page.getByText(name, { exact: true })).toBeVisible();
    await expect(page.locator('span', { hasText: '#e2e' }).first()).toBeVisible();

    await page.goto(`/routes/${routeId}?tab=history`);
    await expect(page.getByText('e2e first deploy').first()).toBeVisible();
    await expect(page.getByText('running', { exact: true })).toBeVisible();
    const archived = await page.request.delete(`/api/v1/routes/${routeId}`, { headers: await csrf(page) });
    expect(archived.status()).toBe(204);
  });
});
