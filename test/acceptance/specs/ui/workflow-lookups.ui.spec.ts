import { expect, test } from '../../lib/Fixtures';
import { FakeCrm, LookupSources } from '../../lib/FakeLookup';
import { JsSteps } from '../../lib/RouteDraft';
import { AccTarget, Profile } from '../../lib/Target';
import { UiTiming } from '../../lib/Ui';

/** UI of the P4 features: lookup sources tab (connections 00001) and the dead-letter Causes view (dead-letters 00001). */
const P4Ui = { lookupsTab: 'Lookup sources', causes: 'Causes', dataBadge: 'Bad data', httpKind: 'HTTP' } as const;

test.describe('P4 UI', () => {
  test('TC-LKS-09 the Connections screen lists lookup sources with kind and target', async ({ page, api, ws }) => {
    test.skip(Profile.target !== AccTarget.Local, 'the fake lookup target runs on the test runner (local stack only)');
    const crm = await new FakeCrm().start();
    try {
      const name = ws.names.of('lksui');
      const res = await api.post(LookupSources.path, LookupSources.http(name, await api.teamId(), crm.baseUrl));
      expect(res.status, res.describe()).toBe(201);
      ws.resources.onCleanup(() => api.delete(LookupSources.item(res.body.id)));
      await page.goto('/connections');
      await page.getByRole('tab', { name: P4Ui.lookupsTab }).click();
      await page.getByLabel('Search name or description').fill(name);
      const row = page.getByRole('row').filter({ hasText: name });
      await expect(row).toBeVisible({ timeout: UiTiming.pageMs });
      await expect(row.getByText(P4Ui.httpKind, { exact: true })).toBeVisible();
      await expect(row.getByText(crm.baseUrl.replace(/^http:\/\//, ''), { exact: false })).toBeVisible();
    } finally {
      await crm.stop();
    }
  });

  test('TC-DLW-14 the Causes view groups dead letters and badges them as bad data', async ({ page, ws }) => {
    const conn = await ws.pipe();
    const id = await ws.runningRoute(ws.draft(conn, [{ topic: ws.names.outTopic, steps: [JsSteps.step(JsSteps.throwOnBad)] }]));
    await ws.kafka.produce(ws.names.inTopic, [1, 2, 3].map((i) => ({ key: `bad-${i}`, value: { id: i, bad: true } })));
    await expect.poll(async () => ((await ws.api.get(`/api/v1/routes/${id}/dlq?limit=10`)).body?.items ?? []).length, { timeout: 60_000 }).toBe(3);
    await page.goto(`/dead-letters?route=${id}&view=causes`);
    const main = page.getByRole('main');
    await expect(main.getByText(P4Ui.dataBadge).first()).toBeVisible({ timeout: UiTiming.pageMs });
    await expect(main.getByText(/bad event <n>/)).toBeVisible();
  });
});
