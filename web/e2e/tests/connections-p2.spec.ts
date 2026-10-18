import { createServer, type Server } from 'node:http';
import type { AddressInfo } from 'node:net';
import { expect, test, type Page } from '@playwright/test';

/**
 * CN-109..CN-111 against the real local API: OAuth fields validate, a connection with a schema registry
 * (mock registry served by this test, basic auth) passes the extra checklist row, and the sensitive toggle saves.
 */
const E2E = {
  bootstrap: process.env.E2E_BOOTSTRAP ?? '127.0.0.1:29092',
  prefix: 'dev-connections-e2e-p2-',
  registryUser: 'sr-user',
  registryPassword: 'sr-pass',
  testTimeoutMs: 60_000,
} as const;

/** Minimal Confluent-compatible registry: GET /subjects with basic auth. */
class MockRegistry {
  private server: Server | null = null;

  async start(): Promise<string> {
    const expected = 'Basic ' + Buffer.from(`${E2E.registryUser}:${E2E.registryPassword}`).toString('base64');
    this.server = createServer((req, res) => {
      res.statusCode = req.headers.authorization === expected ? 200 : 401;
      res.end(res.statusCode === 200 ? '["orders-value"]' : '');
    });
    await new Promise<void>((resolve) => this.server?.listen(0, '127.0.0.1', resolve));
    return `http://127.0.0.1:${(this.server.address() as AddressInfo).port}`;
  }

  async stop(): Promise<void> {
    await new Promise<void>((resolve) => this.server?.close(() => resolve()));
  }
}

async function devLogin(page: Page): Promise<void> {
  await page.goto('/login');
  const button = page.getByRole('button', { name: 'Dev login' });
  await button.waitFor({ timeout: 45_000 });
  await button.click();
  await page.waitForURL((url) => !url.pathname.startsWith('/login'));
}

async function openPanelAtSecurity(page: Page, name: string): Promise<void> {
  await page.goto('/connections');
  await page.getByRole('button', { name: '+ New connection' }).first().click();
  const panel = page.getByRole('dialog');
  await panel.getByPlaceholder('orders-prod').fill(name);
  await panel.getByText('Sensitive data').click();
  await panel.getByRole('button', { name: 'Next' }).click();
  await panel.getByLabel('Bootstrap server').fill(E2E.bootstrap);
  await panel.getByLabel('Bootstrap server').press('Enter');
  await panel.getByRole('button', { name: 'Next' }).click();
}

test.describe('connections P2', () => {
  test.setTimeout(E2E.testTimeoutMs);
  const registry = new MockRegistry();
  let registryUrl = '';

  test.beforeAll(async () => {
    registryUrl = await registry.start();
  });

  test.afterAll(async () => {
    await registry.stop();
  });

  test('OAuth mode shows its fields and server validation', async ({ page }) => {
    await devLogin(page);
    await openPanelAtSecurity(page, `${E2E.prefix}oauth-${Date.now()}`);
    const panel = page.getByRole('dialog');
    await panel.getByRole('radio', { name: 'OAuth' }).click();
    await panel.getByPlaceholder('https://idp.example.com/oauth2/token').fill('idp/token');
    await panel.getByRole('button', { name: 'Test', exact: true }).click();
    await expect(panel.getByText('Enter a full http(s) URL', { exact: false })).toBeVisible();
    await expect(panel.getByText('Enter the OAuth client ID.')).toBeVisible();
  });

  test('schema registry row passes and the sensitive flag is saved', async ({ page }) => {
    const name = `${E2E.prefix}${Date.now()}`;
    await devLogin(page);
    await openPanelAtSecurity(page, name);
    const panel = page.getByRole('dialog');
    await panel.getByRole('radio', { name: 'None' }).click();
    await panel.getByRole('button', { name: 'Next' }).click();
    await panel.getByPlaceholder('https://schema-registry.internal:8081').fill(registryUrl);
    await panel.getByRole('radio', { name: 'Basic' }).click();
    await panel.getByLabel('Username').fill(E2E.registryUser);
    await panel.getByPlaceholder('Password — write-only after saving').fill(E2E.registryPassword);
    await panel.getByRole('button', { name: 'Save & test' }).click();

    await expect(page.getByText('Schema registry reachable')).toBeVisible({ timeout: 30_000 });
    await expect(page.getByText('All checks passed.')).toBeVisible({ timeout: 30_000 });
    await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click();

    const row = page.getByRole('row').filter({ hasText: name });
    await row.getByRole('button', { name: 'Edit' }).click();
    const edit = page.getByRole('dialog');
    await expect(edit.getByRole('checkbox', { name: /Sensitive data/ })).toBeChecked();
    await edit.getByRole('button', { name: '4. Schema registry' }).click();
    await expect(edit.getByPlaceholder('https://schema-registry.internal:8081')).toHaveValue(registryUrl);
    await expect(edit.getByText('••••••')).toBeVisible();
    await edit.getByRole('button', { name: 'Close' }).click();

    await row.getByRole('button', { name: 'Delete' }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Delete' }).click();
    await expect(page.getByRole('row').filter({ hasText: name })).toHaveCount(0);
  });
});
