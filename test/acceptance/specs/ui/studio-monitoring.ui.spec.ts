import { expect, test } from '../../lib/Fixtures';
import { JsSteps } from '../../lib/RouteDraft';
import { Ui, UiTiming } from '../../lib/Ui';

/** UI flows: Transform Studio, dashboard, live tail, dead letters, audit log, settings (UI spec §4, §8, §10, §11.3). */
const Customer = { key: 'c-1', value: { cust_id: 7, email: 'john@mail.com', country: 'VN' } } as const;

test.describe('transform studio UI', () => {
  test('TC-TS-29 adding recipe steps shows a before → after example and the output', async ({ page, ws }) => {
    const ui = new Ui(page);
    const session = await ws.uploadSession([Customer]);
    await page.goto(`/routes/new?step=transform&session=${session}&sample=0`);
    await expect(page.getByText('Event 1 of 1')).toBeVisible({ timeout: UiTiming.pageMs });
    await ui.addStep('Mask');
    await page.getByLabel('Field', { exact: true }).last().selectOption('email');
    await expect(page.getByText(/john@mail\.com → .*j\*\*\*@mail\.com/).first()).toBeVisible({ timeout: UiTiming.previewMs });
    await ui.addStep('Rename');
    await page.getByLabel('Field', { exact: true }).last().selectOption('cust_id');
    await page.getByLabel('Target field', { exact: true }).last().fill('customerId');
    const output = page.locator('section', { has: page.getByText('After all steps') }).last();
    await expect(output).toContainText('customerId', { timeout: UiTiming.previewMs });
    await expect(output).toContainText('j***@mail.com');
  });

  test('TC-TS-31 the JS editor info panel states the sandbox safety rules and limits', async ({ page, ws }) => {
    const ui = new Ui(page);
    const session = await ws.uploadSession([Customer]);
    await page.goto(`/routes/new?step=transform&session=${session}&sample=0`);
    await expect(page.getByText('Event 1 of 1')).toBeVisible({ timeout: UiTiming.pageMs });
    await ui.addStep('JavaScript step');
    await page.getByRole('button', { name: /^ⓘ/ }).first().click();
    const panel = page.getByRole('note').filter({ hasText: 'Safety rules' });
    await expect(panel.getByText(/no network calls/i)).toBeVisible({ timeout: UiTiming.pageMs });
    await expect(panel.getByText(/50 ms per event/)).toBeVisible();
  });
});

test.describe('monitoring UI', () => {
  test('TC-MON-12 the dashboard shows the four health cards and + New route', async ({ page, ws }) => {
    await ws.api.me();
    await page.goto('/');
    const main = page.getByRole('main');
    for (const label of ['Routes', 'Events forwarded · last hour', 'Total consumer lag', 'Dead-letter events · 24 h']) await expect(main.getByText(label, { exact: true })).toBeVisible({ timeout: UiTiming.pageMs });
    await expect(main.getByText('Needs attention', { exact: true })).toBeVisible();
    await expect(main.getByText('Recent activity', { exact: true })).toBeVisible();
    await expect(page.getByRole('button', { name: '+ New route' }).first()).toBeVisible();
  });

  test('TC-MON-11 live tail streams input and output of a running route', async ({ page, ws }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn));
    await page.goto(`/routes/${id}?tab=tail`);
    await expect(page.getByTestId('tail-state')).toHaveText(/Streaming/, { timeout: UiTiming.pageMs });
    const rows = page.getByTestId('tail-row');
    await expect
      .poll(async () => {
        await ws.kafka.produce(ws.names.inTopic, [{ key: `t-${Date.now()}`, value: { tail: 'acc' } }]);
        return rows.count();
      }, { timeout: 60_000, intervals: [3_000] })
      .toBeGreaterThan(0);
    await rows.first().click();
    await expect(page.getByText(new RegExp(`^Input · ${ws.names.inTopic}`))).toBeVisible();
  });

  test('TC-DLQ-11 the Dead letters screen lists failures, shows the original event and retries', async ({ page, ws }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.step(JsSteps.throwOnBad)] }]));
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'bad', value: { id: 9, bad: true } }]);
    await page.goto(`/dead-letters?route=${id}`);
    const main = page.getByRole('main');
    const rows = main.getByRole('checkbox', { name: /Select event at offset/ });
    await expect(async () => {
      await page.reload();
      await expect(rows).toHaveCount(1, { timeout: 5_000 });
    }).toPass({ timeout: 60_000 });
    await main.getByText(/p\d+@\d+/).first().click();
    await expect(main.getByText('Original event', { exact: true })).toBeVisible();
    await main.getByRole('button', { name: 'Retry', exact: true }).click();
    await expect(page.getByRole('dialog').getByText('Retry report')).toBeVisible({ timeout: UiTiming.pageMs });
  });

  test('TC-AUD-08 the audit log screen filters by text and opens an entry', async ({ page, ws }) => {
    await ws.connection();
    await page.goto('/audit');
    await page.getByLabel('Filter audit log').fill(ws.names.connection);
    const rows = page.getByTestId('audit-rows').getByRole('row');
    await expect(rows.filter({ hasText: ws.names.connection }).first()).toBeVisible({ timeout: UiTiming.pageMs });
    await rows.filter({ hasText: ws.names.connection }).first().click();
    await expect(page.getByRole('dialog').getByText(ws.names.connection).first()).toBeVisible();
  });

  test('TC-SET-06 the settings screen shows members & roles and API tokens', async ({ page, ws }) => {
    await ws.api.me();
    await page.goto('/settings');
    await expect(page.getByText('Members & roles').first()).toBeVisible({ timeout: UiTiming.pageMs });
    await expect(page.getByRole('button', { name: 'New token' })).toBeVisible();
  });
});
