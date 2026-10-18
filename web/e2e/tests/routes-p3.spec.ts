import { expect, test } from '@playwright/test';
import { ApiSession, E2EBootstrap } from './ApprovalsFixture';

/**
 * routes#p3-fe-exactly-once: the Settings tab switches a route to exactly-once delivery with the
 * read_committed explanation, warns when the dead-letter topic is on another connection, saves the mode into
 * the draft and the API surfaces the read_committed warning. Names start with `dev-routes-p3e2e-`.
 */
const E2E = { prefix: 'dev-routes-p3e2e-', timeoutMs: 90_000 } as const;

const Text = {
  readCommitted: /isolation\.level=read_committed/,
  atLeastOnce: /after a restart or retry some events can arrive twice/,
  sameConnection: 'Exactly once needs every destination and the dead-letter topic on the source connection.',
} as const;

async function connection(api: ApiSession, name: string): Promise<string> {
  const conn = await api.call<{ id: string }>('POST', '/api/v1/connections', { name, envTag: 'dev', bootstrapServers: [E2EBootstrap], security: { mode: 'plaintext' } });
  expect(conn.status, JSON.stringify(conn.body)).toBe(201);
  return conn.body.id;
}

test.describe('routes P3', () => {
  test.setTimeout(E2E.timeoutMs);

  test('settings: exactly-once delivery with read_committed explanation and same-connection warning', async ({ page }) => {
    const api = new ApiSession(page.request);
    await api.login();
    const name = `${E2E.prefix}${Date.now()}`;
    const source = await connection(api, `${name}-src`);
    const other = await connection(api, `${name}-other`);
    const draft = {
      apiVersion: 'router/v1', name,
      source: { connectionId: source, topics: [`${name}-in`], format: { type: 'json' } },
      branches: [{ id: 'main', destination: { connectionId: source, topic: `${name}-out` } }],
    };
    const created = await api.call<{ id: string }>('POST', '/api/v1/routes', { name, draft });
    expect(created.status, JSON.stringify(created.body)).toBe(201);
    const id = created.body.id;

    await page.goto(`/routes/${id}?tab=settings`);
    const delivery = page.getByRole('radiogroup', { name: 'Delivery mode' });
    await expect(delivery.getByRole('radio', { name: 'At least once' })).toHaveAttribute('aria-checked', 'true');
    await expect(page.getByText(Text.atLeastOnce)).toBeVisible();
    await delivery.getByRole('radio', { name: 'Exactly once' }).click();
    await expect(page.getByText(Text.readCommitted)).toBeVisible();
    await expect(page.getByText(Text.sameConnection)).toHaveCount(0);
    await page.getByLabel('Dead-letter connection').selectOption(other);
    await expect(page.getByText(Text.sameConnection)).toBeVisible();
    await page.getByLabel('Dead-letter connection').selectOption('');
    await expect(page.getByText(Text.sameConnection)).toHaveCount(0);
    await page.getByRole('button', { name: 'Save settings' }).click();
    await expect(page.getByText('Settings saved')).toBeVisible();

    const detail = await api.call<{ draft: { delivery?: { mode: string } } }>('GET', `/api/v1/routes/${id}`);
    expect(detail.body.draft.delivery).toEqual({ mode: 'exactly_once' });
    const validated = await api.call<{ valid: boolean; issues: { code: string; severity: string }[] }>('POST', `/api/v1/routes/${id}/validate`, { mode: 'draft' });
    expect(validated.body.issues).toContainEqual(expect.objectContaining({ code: 'exactly_once_read_committed', severity: 'warning' }));

    await page.reload();
    await expect(page.getByRole('radiogroup', { name: 'Delivery mode' }).getByRole('radio', { name: 'Exactly once' })).toHaveAttribute('aria-checked', 'true');
    expect((await api.call('DELETE', `/api/v1/routes/${id}`)).status).toBe(204);
  });
});
