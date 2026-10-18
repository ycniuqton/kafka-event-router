import { expect, test, type Page } from '@playwright/test';

/**
 * PC-70: Settings → API tokens — create (secret shown once) → the token authenticates the API → revoke → 401.
 * Runs against the local stack (vite with VITE_DATA_SOURCE=http, api with ROUTER_AUTH_MODES including api_token).
 */
const E2E = { prefix: 'dev-platform-core-e2e-', apiURL: process.env.E2E_API_URL ?? 'http://localhost:8080', timeoutMs: 60_000 } as const;

async function devLogin(page: Page): Promise<void> {
  await page.goto('/login');
  const button = page.getByRole('button', { name: 'Dev login' });
  await button.waitFor({ timeout: 45_000 });
  await button.click();
  await page.waitForURL((url) => !url.pathname.startsWith('/login'));
}

test.describe('api tokens', () => {
  test.setTimeout(E2E.timeoutMs);

  test('create → use → revoke', async ({ page, request }) => {
    const name = `${E2E.prefix}${Date.now()}`;
    await devLogin(page);
    await page.goto('/settings');
    await page.getByRole('button', { name: 'New token' }).click();
    const dialog = page.getByRole('dialog');
    await dialog.getByLabel('Name').fill(name);
    await dialog.getByRole('radio', { name: 'write', exact: true }).click();
    await dialog.getByRole('button', { name: 'New token' }).click();
    const secret = (await dialog.getByTestId('api-token-secret').textContent()) ?? '';
    expect(secret.startsWith('ert_')).toBe(true);

    const authorised = await request.get(`${E2E.apiURL}/api/v1/routes`, { headers: { Authorization: `Bearer ${secret}` } });
    expect(authorised.status()).toBe(200);

    await dialog.getByRole('button', { name: 'Done' }).click();
    const row = page.getByText(name);
    await expect(row).toBeVisible();
    await row.locator('xpath=..').getByRole('button', { name: 'Revoke' }).click();
    await page.getByRole('dialog').getByRole('button', { name: 'Revoke' }).click();
    await expect(page.getByText('Token revoked')).toBeVisible();
    await expect(page.getByText(name)).toHaveCount(0);

    const revoked = await request.get(`${E2E.apiURL}/api/v1/routes`, { headers: { Authorization: `Bearer ${secret}` } });
    expect(revoked.status()).toBe(401);
  });
});
