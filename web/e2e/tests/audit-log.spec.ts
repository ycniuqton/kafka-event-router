import { expect, test, type APIRequestContext } from '@playwright/test';
import { readFile } from 'node:fs/promises';
import { AccessSession, Csrf } from './AccessFixture';

/** AC-216: the audit page's sensitive-only filter shows raw-sample views; Export downloads the filtered CSV. */
const Scenario = {
  samples: '/api/v1/preview/samples',
  events: [{ key: 'dev-access-control-audit', value: { orderId: 'dev-ac-1', amount: 1 } }],
  rawView: 'Viewed raw (unmasked) samples',
  sensitiveChip: 'Sensitive only',
  search: 'Filter audit log',
  csvHeader: 'id,at,actor_id,actor_name,actor_kind,action,category,object_type,object_id,object_label,team_id,ip,request_id,sensitive,diff',
} as const;

/** Uploads samples as the signed-in admin: an unmasked view, audited as sample.viewed_raw (sensitive). */
async function uploadSamples(request: APIRequestContext): Promise<string> {
  const csrf = (await request.storageState()).cookies.find((c) => c.name === Csrf.cookie)?.value ?? '';
  const res = await request.post(Scenario.samples, { data: { mode: 'upload', events: Scenario.events }, headers: { [Csrf.header]: csrf } });
  expect(res.ok(), await res.text()).toBeTruthy();
  return (await res.json()).session.id;
}

test('sensitive-only filter shows raw sample views and the detail diff', async ({ browser }) => {
  const context = await browser.newContext();
  await new AccessSession(context.request).login();
  const session = await uploadSamples(context.request);
  const page = await context.newPage();
  await page.goto('/audit');
  await page.getByRole('button', { name: Scenario.sensitiveChip }).click();
  const rows = page.getByTestId('audit-rows').getByRole('row');
  await expect(rows.first()).toBeVisible();
  const total = await rows.count();
  await expect(page.locator('[data-testid="audit-rows"] [role="row"][data-sensitive="true"]')).toHaveCount(total);
  await page.getByLabel(Scenario.search).fill(session);
  const row = rows.filter({ hasText: Scenario.rawView });
  await expect(row).toHaveCount(1);
  await row.click();
  const drawer = page.getByRole('dialog');
  await expect(drawer.getByText(session)).toBeVisible();
  await expect(drawer.getByText('Sensitive: raw data access or a credential change.')).toBeVisible();
  await expect(drawer.getByText('masked:', { exact: true })).toBeVisible();
});

test('export downloads the filtered log as CSV', async ({ browser }) => {
  const context = await browser.newContext({ acceptDownloads: true });
  await new AccessSession(context.request).login();
  const session = await uploadSamples(context.request);
  const page = await context.newPage();
  await page.goto('/audit');
  await page.getByLabel(Scenario.search).fill(session);
  await expect(page.getByTestId('audit-rows').getByRole('row').filter({ hasText: Scenario.rawView })).toHaveCount(1);
  const [download] = await Promise.all([page.waitForEvent('download'), page.getByRole('button', { name: 'Export' }).click()]);
  expect(download.suggestedFilename()).toMatch(/^audit-log-\d{8}-\d{6}\.csv$/);
  const csv = await readFile(await download.path(), 'utf8');
  const lines = csv.trim().split('\n');
  expect(lines[0]).toBe(Scenario.csvHeader);
  expect(lines).toHaveLength(2);
  expect(lines[1]).toContain(session);
  expect(lines[1]).toContain('sample.viewed_raw');
});
