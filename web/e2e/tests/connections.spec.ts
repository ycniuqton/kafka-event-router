import { expect, test, type Page } from '@playwright/test';

/**
 * CN-58: create connection → test → browse topics → create topic → peek → delete, against the real API
 * (baseURL from playwright.config.ts with VITE_DATA_SOURCE=http, dev login, Kafka reachable at E2E_BOOTSTRAP).
 */
const E2E = {
  bootstrap: process.env.E2E_BOOTSTRAP ?? '127.0.0.1:29092',
  prefix: 'dev-connections-e2e-',
  testTimeoutMs: 60_000,
} as const;

async function devLogin(page: Page): Promise<void> {
  await page.goto('/login');
  const button = page.getByRole('button', { name: 'Dev login' });
  await button.waitFor({ timeout: 45_000 });
  await button.click();
  await page.waitForURL((url) => !url.pathname.startsWith('/login'));
}

async function fillPanel(page: Page, name: string): Promise<void> {
  const panel = page.getByRole('dialog');
  await panel.getByPlaceholder('orders-prod').fill(name);
  await panel.getByText('Allow creating topics').click();
  await panel.getByRole('button', { name: 'Next' }).click();
  await panel.getByLabel('Bootstrap server').fill(E2E.bootstrap);
  await panel.getByLabel('Bootstrap server').press('Enter');
  await panel.getByRole('button', { name: 'Next' }).click();
}

test.describe('connections', () => {
  test.setTimeout(E2E.testTimeoutMs);

  test('validation errors jump to the failing step', async ({ page }) => {
    await devLogin(page);
    await page.goto('/connections');
    await page.getByRole('button', { name: '+ New connection' }).first().click();
    await fillPanel(page, `${E2E.prefix}invalid-${Date.now()}`);
    const panel = page.getByRole('dialog');
    await panel.getByRole('radio', { name: 'SASL/PLAIN' }).click();
    await panel.getByRole('button', { name: 'Test', exact: true }).click();
    await expect(panel.getByText('Enter the SASL username.')).toBeVisible();
  });

  test('create → test → topics → create topic → peek → delete', async ({ page }) => {
    const name = `${E2E.prefix}${Date.now()}`;
    await devLogin(page);
    await page.goto('/connections');
    await page.getByRole('button', { name: '+ New connection' }).first().click();
    await fillPanel(page, name);
    const panel = page.getByRole('dialog');
    await panel.getByRole('radio', { name: 'None' }).click();
    await panel.getByRole('button', { name: 'Next' }).click();
    await panel.getByRole('button', { name: 'Save & test' }).click();

    await expect(page.getByText('All checks passed.')).toBeVisible({ timeout: 30_000 });
    await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click();

    const row = page.getByRole('row').filter({ hasText: name });
    await expect(row.getByText('Healthy')).toBeVisible();
    await row.getByRole('button', { name: 'Topics' }).click();
    const topics = page.getByRole('dialog');
    await topics.getByRole('button', { name: '+ Create topic' }).click();
    const topic = `${name}-topic`;
    await page.getByPlaceholder('orders.paid').fill(topic);
    await page.getByRole('button', { name: 'Create topic', exact: true }).click();
    await expect(topics.getByText(topic, { exact: true })).toBeVisible();
    await topics.getByRole('button', { name: 'Peek' }).first().click();
    await expect(topics.getByText(/No events on this topic yet|Reading the latest events|\{/).first()).toBeVisible();
    await topics.getByRole('button', { name: 'Close' }).click();

    await row.getByRole('button', { name: 'Edit' }).click();
    const edit = page.getByRole('dialog');
    await edit.getByRole('textbox').nth(1).fill('edited by e2e');
    await edit.getByRole('button', { name: '4. Schema registry' }).click();
    await edit.getByText('Advanced · raw client properties').click();
    await edit.getByRole('button', { name: '+ Add property' }).click();
    await edit.getByLabel('Property name').last().fill('not.allowed');
    await edit.getByLabel('Property value').last().fill('1');
    await edit.getByRole('button', { name: 'Save & test' }).click();
    await expect(edit.getByText('This property is not supported.', { exact: false })).toBeVisible();
    await edit.getByTitle('Remove property').last().click();
    await edit.getByRole('button', { name: 'Save & test' }).click();
    await expect(page.getByText('Connection updated')).toBeVisible();
    await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click();

    await row.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Delete' }).click();
    await expect(page.getByRole('row').filter({ hasText: name })).toHaveCount(0);
  });
});
