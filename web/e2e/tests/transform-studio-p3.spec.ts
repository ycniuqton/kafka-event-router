import { expect, test, type APIRequestContext, type Page } from '@playwright/test';

/**
 * TS-236 (P3 Studio magic + library) on the local stack: accept a mock AI proposal, infer steps from a pasted
 * example, save steps to the library from the Studio, publish a newer version and update a pinned library step
 * through the "Update available" badge, then find the transform on the library page.
 * Routes and transforms are named `dev-transform-studio-e2e-*`; routes are never deployed and archived at the end.
 */
const P3 = {
  csrfCookie: 'er_csrf',
  csrfHeader: 'X-CSRF-Token',
  timeoutMs: 150_000,
  viewport: { width: 1600, height: 1000 },
  prefix: 'dev-transform-studio-e2e-',
  connection: process.env.E2E_STUDIO_CONNECTION,
  events: [{ key: 'o-1', value: { orderId: 'o-1', email: 'ada@example.com', cust_id: 'c-1', amount: 12 } }],
} as const;

type RouteStep = Record<string, unknown>;

/** API calls of the scenarios (session cookie + CSRF header of the logged-in page). */
class StudioApi {
  constructor(private readonly page: Page) {}

  private async headers(): Promise<Record<string, string>> {
    const cookie = (await this.page.context().cookies()).find((c) => c.name === P3.csrfCookie);
    return { [P3.csrfHeader]: cookie?.value ?? '' };
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

  /** Sets the workspace "AI assist" setting (assist endpoints answer 403 while it is off); returns the previous value. */
  async setAssist(enabled: boolean): Promise<boolean> {
    const current = await this.request.get('/api/v1/workspace-settings');
    const previous: boolean = (await current.json()).aiAssistEnabled;
    const headers = { ...(await this.headers()), 'If-Match': current.headers()['etag'] ?? '' };
    const res = await this.request.patch('/api/v1/workspace-settings', { data: { aiAssistEnabled: enabled }, headers });
    expect(res.ok(), await res.text()).toBeTruthy();
    return previous;
  }

  async uploadSession(): Promise<string> {
    const res = await this.request.post('/api/v1/preview/samples', { data: { mode: 'upload', events: P3.events }, headers: await this.headers() });
    expect(res.ok(), await res.text()).toBeTruthy();
    return (await res.json()).session.id;
  }

  /** A dev connection (bootstrap through the tunnel) to reference from drafts. */
  async connection(): Promise<string> {
    if (P3.connection) return P3.connection;
    const res = await this.request.get('/api/v1/connections');
    const items: { id: string; name: string }[] = (await res.json()).items;
    return (items.find((c) => c.name.startsWith('dev-')) ?? items[0]).id;
  }

  async createRoute(name: string, steps: RouteStep[]): Promise<string> {
    const connectionId = await this.connection();
    const draft = {
      apiVersion: 'router/v1',
      name,
      source: { connectionId, topics: [`${name}-in`], format: { type: 'json' } },
      branches: [{ id: 'main', transform: { steps }, destination: { connectionId, topic: `${name}-out` } }],
    };
    const res = await this.request.post('/api/v1/routes', { data: { name, draft }, headers: await this.headers() });
    expect(res.ok(), await res.text()).toBeTruthy();
    return (await res.json()).id;
  }

  async transformId(name: string): Promise<string> {
    const res = await this.request.get(`/api/v1/transforms?names=${name}`);
    const items: { id: string }[] = (await res.json()).items;
    expect(items.length).toBe(1);
    return items[0].id;
  }

  async publish(transformId: string, steps: RouteStep[]): Promise<void> {
    const res = await this.request.post(`/api/v1/transforms/${transformId}/versions`, { data: { steps, changeNote: 'e2e v2' }, headers: await this.headers() });
    expect(res.ok(), await res.text()).toBeTruthy();
  }

  async createTable(name: string, rows: { key: string; value: unknown }[]): Promise<void> {
    const res = await this.request.post('/api/v1/lookup-tables', { data: { name, keyType: 'string', rows }, headers: await this.headers() });
    expect(res.ok(), await res.text()).toBeTruthy();
  }

  async archive(routeId: string): Promise<void> {
    await this.request.delete(`/api/v1/routes/${routeId}`, { headers: await this.headers() });
  }
}

async function openStudio(page: Page, path: string): Promise<void> {
  await page.goto(path);
  await expect(page.getByText('Event 1 of 1')).toBeVisible({ timeout: 30_000 });
}

test.describe('transform studio P3', () => {
  test.setTimeout(P3.timeoutMs);
  test.use({ viewport: P3.viewport });

  test('AI assist proposal is shown as a diff and applied on accept', async ({ page }) => {
    const api = new StudioApi(page);
    await api.login();
    const previous = await api.setAssist(true);
    try {
      const session = await api.uploadSession();
      await openStudio(page, `/routes/new?step=transform&session=${session}&sample=0`);
      await page.getByLabel('Describe the transform in words').fill('mask email and rename cust_id to customerId');
      await page.getByRole('button', { name: 'Propose' }).click();
      const diff = page.getByLabel('Proposed steps');
      await expect(diff.getByText(/Mask email/)).toBeVisible({ timeout: 15_000 });
      await expect(diff.getByText(/Rename cust_id/)).toBeVisible();
      await diff.getByRole('button', { name: 'Accept' }).click();
      await expect(page.getByText('cust_id → customerId')).toBeVisible();
      await expect(page.getByText(/1 events · 1 ok/)).toBeVisible({ timeout: 20_000 });
    } finally {
      await api.setAssist(previous);
    }
  });

  test('AI assist is refused while the workspace setting is off', async ({ page }) => {
    const api = new StudioApi(page);
    await api.login();
    const previous = await api.setAssist(false);
    try {
      const session = await api.uploadSession();
      await openStudio(page, `/routes/new?step=transform&session=${session}&sample=0`);
      await page.getByLabel('Describe the transform in words').fill('rename cust_id to customerId');
      await page.getByRole('button', { name: 'Propose' }).click();
      await expect(page.getByRole('alert').filter({ hasText: /AI assist is turned off/ })).toBeVisible({ timeout: 15_000 });
    } finally {
      await api.setAssist(previous);
    }
  });

  test('transform by example infers steps with confidence and applies the accepted ones', async ({ page }) => {
    const api = new StudioApi(page);
    await api.login();
    const session = await api.uploadSession();
    await openStudio(page, `/routes/new?step=transform&session=${session}&sample=0`);
    await page.getByRole('button', { name: 'By example' }).click();
    const desired = { orderId: 'o-1', customer: { id: 'c-1' }, total: 12, channel: 'web' };
    await page.getByLabel('Desired output (JSON value)').fill(JSON.stringify(desired));
    await page.getByRole('button', { name: 'Propose steps' }).click();
    const mappings = page.getByLabel('Field mappings');
    await expect(mappings.getByText('customer.id', { exact: true })).toBeVisible({ timeout: 15_000 });
    await expect(mappings.getByText('40%')).toBeVisible();
    const proposals = page.getByLabel('Proposed steps');
    const guess = proposals.locator('label[data-low="true"]');
    await expect(guess).toHaveCount(1);
    await guess.getByRole('checkbox').uncheck();
    await proposals.getByRole('button', { name: /Accept \d+ steps/ }).click();
    await expect(page.getByText('cust_id → customer.id')).toBeVisible();
    await expect(page.getByText(/1 events · 1 ok/)).toBeVisible({ timeout: 20_000 });
  });

  test('save to library, publish v2 and update a pinned library step through the badge', async ({ page }) => {
    const api = new StudioApi(page);
    await api.login();
    const stamp = Date.now();
    const name = `${P3.prefix}lib-${stamp}`;
    const session = await api.uploadSession();
    const source = await api.createRoute(`${P3.prefix}${stamp}-src`, [{ kind: 'rename', from: 'value.cust_id', to: 'value.customerId' }]);
    let user: string | null = null;
    try {
      await openStudio(page, `/routes/${source}/edit?step=transform&session=${session}&sample=0`);
      await page.getByRole('button', { name: 'Save to library' }).click();
      await page.getByRole('dialog').getByRole('textbox').first().fill(name);
      await page.getByRole('button', { name: 'Save', exact: true }).click();
      await expect(page.getByText(`Saved ${name} v1 to the library`)).toBeVisible({ timeout: 15_000 });

      await api.publish(await api.transformId(name), [{ kind: 'mask', field: 'value.email', style: 'email' }]);
      user = await api.createRoute(`${P3.prefix}${stamp}-user`, [{ kind: 'library', ref: name, version: 1 }]);
      await openStudio(page, `/routes/${user}/edit?step=transform&session=${session}&sample=0`);
      const pin = page.getByTestId('library-pin');
      await expect(pin.getByText(`${name} v1`)).toBeVisible({ timeout: 15_000 });
      await expect(pin.getByText('Update available: v2')).toBeVisible({ timeout: 15_000 });
      await pin.getByRole('button', { name: 'Update' }).click();
      await expect(pin.getByText(`${name} v2`)).toBeVisible();
      await expect(pin.getByText('Update available: v2')).toHaveCount(0);
      await expect(page.getByText(/1 events · 1 ok/)).toBeVisible({ timeout: 20_000 });

      await page.goto(`/library?q=${name}`);
      await page.getByRole('button', { name }).click();
      const versions = page.getByLabel('Versions');
      await expect(versions.getByText('v2', { exact: true })).toBeVisible({ timeout: 15_000 });
      await expect(versions.getByText('e2e v2')).toBeVisible();
    } finally {
      await api.archive(source);
      if (user) await api.archive(user);
    }
  });

  test('lookup recipe reads a named library table pinned to a version', async ({ page }) => {
    const api = new StudioApi(page);
    await api.login();
    const table = `${P3.prefix}regions-${Date.now()}`;
    await api.createTable(table, [{ key: 'c-1', value: 'EMEA' }]);
    const session = await api.uploadSession();
    await openStudio(page, `/routes/new?step=transform&session=${session}&sample=0`);
    await page.getByRole('button', { name: '+ Add step' }).click();
    await page.getByLabel('Search steps').fill('Lookup');
    await page.getByRole('button', { name: 'Lookup', exact: true }).click();
    await page.getByLabel('Field').last().selectOption('cust_id');
    await page.getByLabel('Write to (empty = replace the key)').fill('region');
    await page.getByRole('radio', { name: 'Library table' }).click();
    await page.getByLabel('Library object').selectOption(table);
    await expect(page.getByText(`${table} v1`).first()).toBeVisible();
    await expect(page.getByText(/1 events · 1 ok/)).toBeVisible({ timeout: 20_000 });
    await expect(page.getByText(/region:/).first()).toBeVisible({ timeout: 20_000 });
  });

  test('library page: create a lookup table, refuse a bad CSV line by line, publish a fixed CSV', async ({ page }) => {
    const api = new StudioApi(page);
    await api.login();
    const table = `${P3.prefix}codes-${Date.now()}`;
    await page.goto('/library?tab=lookup-tables');
    await page.getByRole('button', { name: 'New lookup table' }).click();
    const create = page.getByRole('dialog');
    await create.getByRole('textbox').first().fill(table);
    await create.getByLabel('Rows (CSV with a key column)').fill('key,value\nA,1');
    await create.getByRole('button', { name: 'Create' }).click();
    await expect(page.getByText(`Created ${table} v1`)).toBeVisible({ timeout: 15_000 });
    await page.getByRole('button', { name: 'Import CSV' }).click();
    const dialog = page.getByRole('dialog', { name: /Import CSV/ }).or(page.getByRole('dialog').last());
    await dialog.getByLabel('or paste the CSV').fill('key,value\nB,2\nB,3\n,4');
    await dialog.getByRole('button', { name: 'Publish version' }).click();
    await expect(dialog.getByText('Line 3: Duplicate key "B" (first seen on line 2).')).toBeVisible({ timeout: 15_000 });
    await expect(dialog.getByText('Line 4: The key is empty.')).toBeVisible();
    await dialog.getByLabel('or paste the CSV').fill('key,value\nB,2\nC,3');
    await dialog.getByRole('button', { name: 'Publish version' }).click();
    await expect(page.getByText(`Published ${table} v2 (2 rows)`)).toBeVisible({ timeout: 15_000 });
    await expect(page.getByLabel('Rows').getByText('C', { exact: true })).toBeVisible({ timeout: 15_000 });
  });
});
