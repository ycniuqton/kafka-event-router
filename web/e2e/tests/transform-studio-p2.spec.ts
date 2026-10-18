import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

/**
 * TS-124 + P2 Studio flows on the local stack: pin a sample as a test case on a saved route, break it with an
 * edit (fail badge + diff), and see deploy blocked by validation; Visual Mapper and Export to JS on a new route.
 * Routes are named `dev-transform-studio-e2e-<ts>`, never deployed, and archived at the end.
 */
const P2 = {
  csrfCookie: 'er_csrf',
  csrfHeader: 'X-CSRF-Token',
  timeoutMs: 120_000,
  viewport: { width: 1600, height: 1000 },
  routePrefix: 'dev-transform-studio-e2e-',
  connection: process.env.E2E_STUDIO_CONNECTION ?? 'a9ff5004-200e-4f42-a6ee-e942f651eff2',
  events: [{ key: 'o-1', value: { orderId: 'o-1', email: 'ada@example.com', cust_id: 'c-1', amount: 12 } }],
} as const;

/** API calls of the scenario (session cookie + CSRF header of the logged-in page). */
class StudioApi {
  constructor(private readonly page: Page) {}

  private async headers(): Promise<Record<string, string>> {
    const cookie = (await this.page.context().cookies()).find((c) => c.name === P2.csrfCookie);
    return { [P2.csrfHeader]: cookie?.value ?? '' };
  }

  private get request(): APIRequestContext {
    return this.page.request;
  }

  async login(): Promise<void> {
    await this.page.goto('/login');
    const button = this.page.getByRole('button', { name: 'Dev login' });
    await button.waitFor({ timeout: 45_000 });
    await button.click();
    await this.page.waitForURL((url) => !url.pathname.startsWith('/login'));
  }

  async uploadSession(): Promise<string> {
    const res = await this.request.post('/api/v1/preview/samples', { data: { mode: 'upload', events: P2.events }, headers: await this.headers() });
    expect(res.ok(), await res.text()).toBeTruthy();
    return (await res.json()).session.id;
  }

  /** A saved route whose draft renames cust_id → customerId. */
  async createRoute(name: string): Promise<string> {
    const draft = {
      apiVersion: 'router/v1',
      name,
      source: { connectionId: P2.connection, topics: [`${name}-in`], format: { type: 'json' } },
      branches: [{ id: 'main', transform: { steps: [{ kind: 'rename', from: 'value.cust_id', to: 'value.customerId' }] }, destination: { connectionId: P2.connection, topic: `${name}-out` } }],
    };
    const res = await this.request.post('/api/v1/routes', { data: { name, draft }, headers: await this.headers() });
    expect(res.ok(), await res.text()).toBeTruthy();
    return (await res.json()).id;
  }

  async validateDeploy(routeId: string): Promise<{ valid: boolean; issues: { code: string; severity: string }[] }> {
    const res = await this.request.post(`/api/v1/routes/${routeId}/validate`, { data: { mode: 'deploy' }, headers: await this.headers() });
    expect(res.ok(), await res.text()).toBeTruthy();
    return res.json();
  }

  async archive(routeId: string): Promise<void> {
    await this.request.delete(`/api/v1/routes/${routeId}`, { headers: await this.headers() });
  }
}

async function addStep(page: Page, label: string): Promise<void> {
  await page.getByRole('button', { name: '+ Add step' }).click();
  await page.getByLabel('Search steps').fill(label);
  await page.getByRole('button', { name: label, exact: true }).click();
}

test.describe('transform studio P2', () => {
  test.setTimeout(P2.timeoutMs);
  test.use({ viewport: P2.viewport });

  test('pinned test case fails after an edit and blocks deploy', async ({ page }) => {
    const api = new StudioApi(page);
    await api.login();
    const routeId = await api.createRoute(`${P2.routePrefix}${Date.now()}`);
    try {
      const session = await api.uploadSession();
      await page.goto(`/routes/${routeId}/edit?step=transform&session=${session}&sample=0`);
      await expect(page.getByText('Event 1 of 1')).toBeVisible({ timeout: 30_000 });
      const panel = page.getByTestId('test-cases');
      await expect(panel.getByRole('button', { name: '+ Pin current event' })).toBeEnabled({ timeout: 30_000 });
      await panel.getByRole('button', { name: '+ Pin current event' }).click();
      const row = panel.getByTestId('test-case');
      await expect(row).toHaveCount(1);
      await expect(row.getByText('Pass', { exact: true })).toBeVisible({ timeout: 20_000 });

      await page.getByText('cust_id → customerId').click();
      await page.getByLabel('Target field').fill('clientId');
      await expect(row.getByText('Fail', { exact: true })).toBeVisible({ timeout: 20_000 });
      await row.getByRole('button', { name: 'Diff' }).click();
      await expect(row.getByText('/value/clientId')).toBeVisible();

      await page.keyboard.press('ControlOrMeta+S');
      await expect.poll(async () => (await api.validateDeploy(routeId)).issues.some((i) => i.code === 'test_case_failed' && i.severity === 'error'), { timeout: 20_000 }).toBe(true);
      expect((await api.validateDeploy(routeId)).valid).toBe(false);

      await row.getByRole('button', { name: 'Accept actual' }).click();
      await expect(row.getByText('Pass', { exact: true })).toBeVisible({ timeout: 20_000 });
      await expect.poll(async () => (await api.validateDeploy(routeId)).issues.some((i) => i.code === 'test_case_failed'), { timeout: 20_000 }).toBe(false);
    } finally {
      await api.archive(routeId);
    }
  });

  test('visual mapper maps chosen fields and greys the rest', async ({ page }) => {
    const api = new StudioApi(page);
    await api.login();
    const session = await api.uploadSession();
    await page.goto(`/routes/new?step=transform&session=${session}&sample=0`);
    await expect(page.getByText('Event 1 of 1')).toBeVisible({ timeout: 30_000 });

    await addStep(page, 'Visual mapper');
    const mapper = page.getByTestId('visual-mapper');
    await mapper.getByRole('radio', { name: 'Empty' }).click();
    const input = mapper.getByTestId('mapper-input');
    await input.locator('[data-path="email"]').getByRole('button', { name: 'Map to output' }).click();
    await input.locator('[data-path="orderId"]').getByRole('button', { name: 'Map to output' }).click();
    await expect(mapper.getByText('2 input fields are not in the output')).toBeVisible();
    await expect(input.locator('[data-path="amount"]')).toHaveAttribute('data-unmapped', 'true');
    await expect(mapper.getByTestId('mapper-output').getByRole('textbox', { name: 'Output field name' })).toHaveCount(2);
    await expect(page.getByText(/1 events · 1 ok/)).toBeVisible({ timeout: 15_000 });
  });

  test('export to JS replaces recipe steps with one JavaScript step', async ({ page }) => {
    const api = new StudioApi(page);
    await api.login();
    const session = await api.uploadSession();
    await page.goto(`/routes/new?step=transform&session=${session}&sample=0`);
    await expect(page.getByText('Event 1 of 1')).toBeVisible({ timeout: 30_000 });
    await page.getByText('cust_id:').first().click();
    await page.getByRole('button', { name: 'Rename', exact: true }).click();
    await page.getByRole('button', { name: 'Export to JS' }).click();
    await page.getByRole('checkbox').first().check();
    await page.getByRole('button', { name: 'Convert to JS' }).click();
    await expect(page.getByText('JavaScript', { exact: true }).first()).toBeVisible({ timeout: 15_000 });
    await expect(page.getByText(/1 events · 1 ok/)).toBeVisible({ timeout: 20_000 });
  });
});
