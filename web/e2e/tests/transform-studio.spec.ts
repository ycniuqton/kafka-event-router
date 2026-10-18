import { expect, test, type Page } from '@playwright/test';

/**
 * TS-28: Transform Studio against the real preview service. Uploads two order events into a sample session,
 * opens the builder's Transform step on it (`?step=transform&session=`), adds Rename (click-to-transform),
 * Mask and a JS step that fails on one event, then checks the output diff and the jump-to-error flow.
 * Nothing is written to Kafka; the draft route is never saved.
 */
const Studio = {
  csrfCookie: 'er_csrf',
  csrfHeader: 'X-CSRF-Token',
  timeoutMs: 90_000,
  wideViewport: { width: 1600, height: 1000 },
  libraryPrefix: 'dev-transform-studio-e2e-',
  failingCode: 'function transform(event, ctx) {\n  ctx.log("amount", event.value.amount);\n  if (event.value.bad) return event.value.missing.field;\n  return event;\n}',
  events: [
    { key: 'o-1', value: { orderId: 'o-1', email: 'ada@example.com', cust_id: 'c-1', amount: 12 } },
    { key: 'o-2', value: { orderId: 'o-2', email: 'bob@example.com', cust_id: 'c-2', amount: -3, bad: true } },
  ],
} as const;

/** Browser session helpers of the scenario. */
class StudioSession {
  constructor(private readonly page: Page) {}

  async login(): Promise<void> {
    await this.page.goto('/login');
    const button = this.page.getByRole('button', { name: 'Dev login' });
    await button.waitFor({ timeout: 45_000 });
    await button.click();
    await this.page.waitForURL((url) => !url.pathname.startsWith('/login'));
  }

  async uploadSession(): Promise<string> {
    const cookie = (await this.page.context().cookies()).find((c) => c.name === Studio.csrfCookie);
    const res = await this.page.request.post('/api/v1/preview/samples', {
      data: { mode: 'upload', events: Studio.events },
      headers: { [Studio.csrfHeader]: cookie?.value ?? '' },
    });
    expect(res.ok(), await res.text()).toBeTruthy();
    return (await res.json()).session.id;
  }

  async addStep(label: string): Promise<void> {
    await this.page.getByRole('button', { name: '+ Add step' }).click();
    await this.page.getByLabel('Search steps').fill(label);
    await this.page.getByRole('button', { name: label, exact: true }).click();
  }
}

test.describe('transform studio', () => {
  test.setTimeout(Studio.timeoutMs);
  test.use({ viewport: Studio.wideViewport });

  test('rename + mask + JS on real samples: output diff and jump to error', async ({ page }) => {
    const studio = new StudioSession(page);
    await studio.login();
    const session = await studio.uploadSession();
    await page.goto(`/routes/new?step=transform&session=${session}&sample=0`);
    await expect(page.getByText('Event 1 of 2')).toBeVisible({ timeout: 30_000 });

    await page.getByText('cust_id:').first().click();
    await page.getByRole('button', { name: 'Rename', exact: true }).click();
    await expect(page.getByText('cust_id → custId')).toBeVisible();

    await studio.addStep('Mask');
    await page.getByLabel('Field').last().selectOption('email');
    await expect(page.getByText(/\*+@example\.com/).first()).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText(/••••••/)).toHaveCount(0);
    await expect(page.locator('[data-mark="+"], :text("custId")').first()).toBeVisible();

    await studio.addStep('JavaScript step');
    const editor = page.locator('.monaco-editor').last();
    await editor.waitFor({ timeout: 20_000 });
    await page.context().grantPermissions(['clipboard-read', 'clipboard-write']);
    await page.evaluate((code) => navigator.clipboard.writeText(code), Studio.failingCode);
    await editor.click();
    await page.keyboard.press('ControlOrMeta+A');
    await page.keyboard.press('ControlOrMeta+V');
    await expect(page.getByText(/1 error/)).toBeVisible({ timeout: 20_000 });
    await page.getByRole('button', { name: 'Jump to #2' }).click();
    await expect(page.getByText('Event 2 of 2')).toBeVisible();
    await expect(page.getByText(/Event #2:/).first()).toBeVisible();
  });

  test('saves the steps to the library and lists them with team name and version', async ({ page }) => {
    const studio = new StudioSession(page);
    await studio.login();
    const session = await studio.uploadSession();
    const name = `${Studio.libraryPrefix}${Date.now()}`;
    await page.goto(`/routes/new?step=transform&session=${session}&sample=0`);
    await page.getByText('email:').first().click();
    await page.getByRole('button', { name: 'Mask', exact: true }).click();
    await page.getByRole('button', { name: 'Save to library' }).click();
    await page.getByRole('dialog').getByRole('textbox').first().fill(name);
    await page.getByRole('dialog').getByRole('button', { name: 'Save' }).click();
    await expect(page.getByText(`Saved ${name} v1 to the library`)).toBeVisible();
    await page.goto('/library');
    const row = page.getByText(name).locator('..');
    await expect(row).toContainText('v1');
    await expect(row).toContainText('1 step');
    await expect(row).toContainText('default');
  });
});
