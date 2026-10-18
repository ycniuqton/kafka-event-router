import { expect, test } from '@playwright/test';
import { AccessSession } from './AccessFixture';

/** AC-42: an admin demotes a member to Editor; after a refetch the member's UI loses admin-only actions. */
const Scenario = {
  memberEmail: `dev-access-control-e2e-${Date.now()}@local.dev`,
  editorRole: 'editor',
  newConnection: '+ New connection',
  auditNav: 'Audit log',
} as const;

test('demoted editor loses admin actions after refetch', async ({ browser }) => {
  const adminContext = await browser.newContext();
  const memberContext = await browser.newContext();
  const admin = new AccessSession(adminContext.request);
  const member = new AccessSession(memberContext.request);
  await admin.login();
  await member.login(Scenario.memberEmail);
  const page = await memberContext.newPage();

  await page.goto('/connections');
  await expect(page.getByRole('button', { name: Scenario.newConnection }).first()).toBeVisible();
  await expect(page.getByRole('link', { name: Scenario.auditNav })).toBeVisible();

  const me = await member.me();
  await admin.demote(me.id, me.teams[0].id, Scenario.editorRole);

  await page.reload();
  await expect(page.getByRole('link', { name: 'Connections' })).toBeVisible();
  await expect(page.getByRole('button', { name: Scenario.newConnection })).toHaveCount(0);
  await expect(page.getByRole('link', { name: Scenario.auditNav })).toHaveCount(0);

  await page.goto('/audit');
  await expect(page.getByText('Only workspace admins can read the audit log.')).toBeVisible();
});

test('workspace admin reads the audit log with names from the real API', async ({ browser }) => {
  const context = await browser.newContext();
  await new AccessSession(context.request).login();
  const page = await context.newPage();
  await page.goto('/audit');
  await expect(page.getByTestId('audit-rows').getByText('Signed in').first()).toBeVisible();
  await page.getByLabel('Filter audit log').fill('zzz-no-such-object');
  await expect(page.getByText('No actions match these filters.')).toBeVisible();
});
