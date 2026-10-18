import { expect, test } from '../../lib/Fixtures';
import { Paths } from '../../lib/Platform';
import { Profile } from '../../lib/Target';
import { Ui, UiTiming } from '../../lib/Ui';

/** UI flows: connections panel, routes list and chooser, builder → deploy, sample panel, history (UI spec §5–§7, §9, §11.1). */
const Samples = ['{"id":1,"type":"OrderPaid","amount":120}', '{"id":2,"type":"OrderCreated","amount":40}', '{"id":3,"type":"OrderPaid","amount":7}'].join('\n');

test.describe('connections UI', () => {
  test('TC-CONN-14 create a connection in the 4-step side panel, test it and see it healthy', async ({ page, api, ws }) => {
    const name = ws.names.connection;
    const ui = new Ui(page);
    await page.goto('/connections');
    await page.getByRole('button', { name: '+ New connection' }).first().click();
    const panel = page.getByRole('dialog');
    await panel.getByPlaceholder('orders-prod').fill(name);
    await panel.getByText('Allow creating topics').click();
    await panel.getByRole('button', { name: 'Next' }).click();
    await panel.getByLabel('Bootstrap server').fill(Profile.platformBootstrap);
    await panel.getByLabel('Bootstrap server').press('Enter');
    await panel.getByRole('button', { name: 'Next' }).click();
    await panel.getByRole('radio', { name: 'None' }).click();
    await panel.getByRole('button', { name: 'Next' }).click();
    await panel.getByRole('button', { name: 'Save & test' }).click();
    await expect(page.getByText('All checks passed.')).toBeVisible({ timeout: UiTiming.pageMs });
    const created = (await api.get(`${Paths.connections}?q=${name}`)).body.items[0];
    ws.resources.connection(created.id);
    await page.getByRole('dialog').getByRole('button', { name: 'Close' }).click();
    await expect(ui.page.getByRole('row').filter({ hasText: name }).getByText('Healthy')).toBeVisible();
  });

  test('TC-CONN-15 Peek shows the latest events of a topic', async ({ page, ws }) => {
    await ws.pipe();
    await ws.kafka.produce(ws.names.inTopic, [{ key: 'peek-1', value: { peekMarker: 'acc-peek' } }]);
    await page.goto('/connections');
    const row = page.getByRole('row').filter({ hasText: ws.names.connection });
    await row.getByRole('button', { name: 'Topics' }).click();
    const topics = page.getByRole('dialog');
    await topics.getByLabel('Search topics').fill(ws.names.inTopic);
    await topics.getByRole('button', { name: 'Peek' }).first().click();
    await expect(topics.getByText(/acc-peek/).first()).toBeVisible({ timeout: UiTiming.pageMs });
  });
});

test.describe('routes UI', () => {
  test('TC-RTL-08 + New route offers the four starting points; the list reads like a sentence', async ({ page, ws }) => {
    const conn = await ws.pipe();
    await ws.createRoute(ws.draft(conn));
    await page.goto('/routes');
    await page.getByRole('button', { name: '+ New route' }).first().click();
    const chooser = page.getByRole('dialog');
    for (const title of ['Filter & forward', 'Publish to partner', 'Merge topics', 'Blank route']) await expect(chooser.getByText(title, { exact: true })).toBeVisible();
    await page.keyboard.press('Escape');
    await page.goto(`/routes?q=${ws.names.route}`);
    const row = page.getByRole('row').filter({ hasText: ws.names.route });
    await expect(row.getByText(ws.names.inTopic).first()).toBeVisible();
    await expect(row.getByText(ws.names.outTopic).first()).toBeVisible();
    await expect(row.getByText(/Draft/i).first()).toBeVisible();
  });

  test('TC-BLD-10 build a route in the builder (source → filter → destination), deploy it and see it running; TC-BLD-12 quick filter from data; TC-BLD-13 live match counter', async ({ page, ws }) => {
    await ws.pipe();
    const ui = new Ui(page);
    await page.goto('/routes/new?template=filter');
    await ui.builderSource(ws.names.route, ws.names.connection, ws.names.inTopic);
    await page.getByRole('button', { name: /^Filter/ }).first().click();
    await page.getByRole('button', { name: 'Remove condition' }).first().click();
    await ui.uploadSamples(Samples);
    await expect(page.getByText(/3 loaded/)).toBeVisible({ timeout: UiTiming.previewMs });
    await page.getByText('"OrderPaid"').first().click();
    await expect(page.getByText(/Matches 2 of 3 sample events/), 'TC-BLD-13 live match counter').toBeVisible({ timeout: UiTiming.previewMs });
    await ui.builderDestination(ws.names.outTopic);
    const id = ws.resources.route(await ui.saveAndDeploy('acc ui deploy'));
    await ws.waitObserved(id, 'running');
    await ws.kafka.produce(ws.names.inTopic, Samples.split('\n').map((value, i) => ({ key: `k${i}`, value })));
    expect((await ws.kafka.consume(ws.names.outTopic, 2)).map((r) => r.value.id)).toEqual([1, 3]);
  });

  test('TC-PRV-12 the sample panel shows loaded / pass / filtered / error counts', async ({ page, ws }) => {
    await ws.pipe();
    const ui = new Ui(page);
    await page.goto('/routes/new?template=filter');
    await ui.builderSource(ws.names.route, ws.names.connection, ws.names.inTopic);
    await page.getByRole('button', { name: /^Filter/ }).first().click();
    await page.getByRole('button', { name: 'Remove condition' }).first().click();
    await ui.uploadSamples(Samples);
    await page.getByText('"OrderPaid"').first().click();
    await expect(page.getByTestId('sample-summary')).toContainText('3 loaded · 2 pass · 1 filtered · 0 errors', { timeout: UiTiming.previewMs });
  });

  test('TC-DEP-11 the History tab lists versions and rolls back to an older one', async ({ page, ws }) => {
    const conn = await ws.pipe();
    const id = await ws.createRoute(ws.draft(conn));
    await ws.deploy(id, 'acc v1');
    await ws.saveDraft(id, ws.draft(conn, [{ topic: ws.names.inTopic }]));
    await ws.deploy(id, 'acc v2');
    await page.goto(`/routes/${id}?tab=history`);
    await expect(page.getByText('acc v1').first()).toBeVisible({ timeout: UiTiming.pageMs });
    await expect(page.getByText('acc v2').first()).toBeVisible();
    await page.getByRole('button', { name: 'Roll back', exact: true }).first().click();
    await page.getByRole('button', { name: 'Roll back to v1' }).click();
    await expect.poll(async () => (await ws.route(id)).activeVersion?.no, { timeout: UiTiming.pageMs }).toBe(3);
  });
});
