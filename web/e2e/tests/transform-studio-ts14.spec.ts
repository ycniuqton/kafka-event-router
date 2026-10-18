import { expect, test, type Locator, type Page } from '@playwright/test';
import kafkajs, { type IHeaders } from 'kafkajs';
import SnappyCodec from 'kafkajs-snappy';

const { CompressionCodecs, CompressionTypes, Kafka, logLevel } = kafkajs;

/** The workers compress output batches with snappy. */
CompressionCodecs[CompressionTypes.Snappy] = SnappyCodec;

/**
 * TS-14: Structure (split array, wrap, unwrap) and Kafka header recipe forms (add, remove, rename, header ↔ payload
 * copy) against the real preview service, then a dev route built with split array + wrap + header steps in the Studio,
 * deployed from the builder and checked on Kafka (produce to `<name>-in`, consume `<name>-out`).
 * Everything created is named `dev-ts14-*`; the route is paused + archived, its connection and topics deleted.
 */
const TS14 = {
  csrfCookie: 'er_csrf',
  csrfHeader: 'X-CSRF-Token',
  timeoutMs: 180_000,
  previewMs: 20_000,
  kafkaMs: 90_000,
  /** The paused worker leaves the route's consumer group shortly after the pause; group deletes are retried meanwhile. */
  groupDeleteAttempts: 10,
  groupDeleteWaitMs: 1_000,
  viewport: { width: 1600, height: 1000 },
  prefix: 'dev-ts14-',
  bootstrap: process.env.E2E_BOOTSTRAP ?? '127.0.0.1:29092',
  event: {
    key: 'o-1',
    headers: [
      { key: 'traceId', value: 't-1' },
      { key: 'x-internal', value: 'yes' },
    ],
    value: { orderId: 'o-1', items: [{ sku: 'A', qty: 1 }, { sku: 'B', qty: 2 }], data: { order: { id: 7 } } },
  },
} as const;

/** Logged-in page, API calls (CSRF double-submit) and Studio interactions of the scenarios. */
class Ts14Session {
  constructor(private readonly page: Page) {}

  async login(): Promise<void> {
    await this.page.goto('/login');
    const button = this.page.getByRole('button', { name: 'Dev login' });
    await button.waitFor({ timeout: 45_000 });
    await button.click();
    await this.page.waitForURL((url) => !url.pathname.startsWith('/login'));
  }

  async headers(etag?: string): Promise<Record<string, string>> {
    const cookie = (await this.page.context().cookies()).find((c) => c.name === TS14.csrfCookie);
    return { [TS14.csrfHeader]: cookie?.value ?? '', ...(etag ? { 'If-Match': etag } : {}) };
  }

  async uploadSession(): Promise<string> {
    const res = await this.page.request.post('/api/v1/preview/samples', { data: { mode: 'upload', events: [TS14.event] }, headers: await this.headers() });
    expect(res.ok(), await res.text()).toBeTruthy();
    return (await res.json()).session.id;
  }

  async openStudio(path: string): Promise<void> {
    await this.page.goto(path);
    await expect(this.page.getByText('Event 1 of 1')).toBeVisible({ timeout: 30_000 });
  }

  async addStep(label: string): Promise<void> {
    await this.page.getByRole('button', { name: '+ Add step' }).click();
    await this.page.getByLabel('Search steps').fill(label);
    await this.page.getByRole('button', { name: label, exact: true }).click();
  }

  /** Fills the last input with this accessible label (the newest step's inline settings). */
  async fill(label: string, text: string): Promise<void> {
    await this.page.getByLabel(label, { exact: true }).last().fill(text);
  }

  async pick(label: string, option: string): Promise<void> {
    await this.page.getByLabel(label, { exact: true }).last().selectOption(option);
  }

  get outputHeaders(): Locator {
    return this.page.locator('div', { hasText: /^headers:/ }).last();
  }

  get output(): Locator {
    return this.page.locator('section', { has: this.page.getByText('After all steps') }).last();
  }

  async previewSettled(): Promise<void> {
    await expect(this.page.locator('[aria-busy="true"]')).toHaveCount(0, { timeout: TS14.previewMs });
    await expect(this.page.getByText(/1 events · 1 ok/)).toBeVisible({ timeout: TS14.previewMs });
  }
}

/** One recipe of the Studio check: how to fill its form and what the preview service must output. */
type RecipeCase = { label: string; fill(s: Ts14Session): Promise<void>; expect(s: Ts14Session, page: Page): Promise<void> };

const recipes: RecipeCase[] = [
  {
    label: 'Split array',
    fill: (s) => s.fill('Item field', 'item'),
    expect: async (s, page) => {
      await expect(page.getByText('items[] → item · one event per item')).toBeVisible();
      await expect(page.getByText('item: {"qty":1,"sku":"A"}', { exact: true })).toBeVisible({ timeout: TS14.previewMs });
      await expect(page.getByText('items: 2 items')).toBeVisible();
      await expect(s.output).toContainText('item');
    },
  },
  {
    label: 'Wrap',
    fill: (s) => s.fill('Wrap in', 'payload'),
    expect: async (s, page) => {
      await expect(page.getByText('value → payload')).toBeVisible();
      await expect(page.getByText(/^payload: \{"data":/)).toBeVisible({ timeout: TS14.previewMs });
      await expect(s.output).toContainText('payload');
    },
  },
  {
    label: 'Unwrap',
    fill: (s) => s.pick('Unwrap field', 'data.order'),
    expect: async (s, page) => {
      await expect(page.getByText('value: {"id":7}', { exact: true })).toBeVisible({ timeout: TS14.previewMs });
      await expect(s.output).toContainText(/\+\s*id:/);
      await expect(s.output).toContainText('→ 8 B');
    },
  },
  {
    label: 'Add header',
    fill: async (s) => {
      await s.fill('Header', 'source');
      await s.fill('Fixed value', 'ts14');
    },
    expect: async (s, page) => {
      await expect(page.getByText(/^source \(no header\) → source: ts14$/)).toBeVisible({ timeout: TS14.previewMs });
      await expect(s.outputHeaders).toContainText('source=ts14');
    },
  },
  {
    label: 'Remove header',
    fill: (s) => s.fill('Header', 'x-internal'),
    expect: async (s, page) => {
      await expect(page.getByText('x-internal (no header)')).toBeVisible({ timeout: TS14.previewMs });
      await expect(s.outputHeaders).not.toContainText('x-internal');
    },
  },
  {
    label: 'Rename header',
    fill: async (s) => {
      await s.fill('Header', 'traceId');
      await s.fill('New header', 'trace-id');
    },
    expect: async (s, page) => {
      await expect(page.getByText(/^traceId: t-1 → trace-id: t-1$/)).toBeVisible({ timeout: TS14.previewMs });
      await expect(s.outputHeaders).toContainText('trace-id=t-1');
      await expect(s.outputHeaders).not.toContainText('traceId=');
    },
  },
  {
    label: 'Copy header to payload',
    fill: async (s) => {
      await s.fill('Header', 'traceId');
      await s.fill('Target field', 'meta.traceId');
    },
    expect: async (s, page) => {
      await expect(page.getByText('meta.traceId: t-1', { exact: true })).toBeVisible({ timeout: TS14.previewMs });
      await expect(s.output).toContainText('meta');
    },
  },
  {
    label: 'Copy payload to header',
    fill: async (s) => {
      await s.pick('Field', 'orderId');
      await s.fill('Header', 'order-id');
    },
    expect: async (s, page) => {
      await expect(page.getByText(/^orderId: o-1 → order-id: o-1$/)).toBeVisible({ timeout: TS14.previewMs });
      await expect(s.outputHeaders).toContainText('order-id=o-1');
    },
  },
];

/** Dev connection + topics through the API, Kafka produce/consume through the tunnel, cleanup. */
class Ts14Kafka {
  private readonly kafka = new Kafka({ clientId: 'dev-ts14-e2e', brokers: [TS14.bootstrap], logLevel: logLevel.NOTHING });

  constructor(
    private readonly page: Page,
    private readonly session: Ts14Session,
    readonly name: string,
  ) {}

  async createConnection(): Promise<string> {
    const headers = await this.session.headers();
    const res = await this.page.request.post('/api/v1/connections', {
      headers,
      data: {
        name: `${this.name}-conn`,
        envTag: 'dev',
        owner: { teamId: await this.teamId() },
        bootstrapServers: [TS14.bootstrap],
        security: { mode: 'plaintext' },
        sensitive: false,
        allowTopicCreate: true,
      },
    });
    expect(res.ok(), await res.text()).toBeTruthy();
    const { id } = await res.json();
    for (const topic of [`${this.name}-in`, `${this.name}-out`]) {
      const created = await this.page.request.post(`/api/v1/connections/${id}/topics`, { headers, data: { name: topic, partitions: 1 } });
      expect(created.ok(), await created.text()).toBeTruthy();
    }
    return id;
  }

  /** The dev user's team: owning the connection gives it raw (unmasked) sample visibility. */
  private async teamId(): Promise<string> {
    const me = await (await this.page.request.get('/api/v1/auth/me')).json();
    return Object.keys(me.access.teams)[0];
  }

  async createRoute(connectionId: string): Promise<string> {
    const draft = {
      apiVersion: 'router/v1',
      name: this.name,
      source: { connectionId, topics: [`${this.name}-in`], format: { type: 'json' }, startFrom: { mode: 'earliest' } },
      branches: [{ id: 'main', transform: { steps: [] }, destination: { connectionId, topic: `${this.name}-out`, format: { type: 'json' } } }],
    };
    const res = await this.page.request.post('/api/v1/routes', { data: { name: this.name, draft }, headers: await this.session.headers() });
    expect(res.ok(), await res.text()).toBeTruthy();
    return (await res.json()).id;
  }

  async produce(): Promise<void> {
    const producer = this.kafka.producer();
    await producer.connect();
    const headers = Object.fromEntries(TS14.event.headers.map((h) => [h.key, h.value]));
    await producer.send({ topic: `${this.name}-in`, messages: [{ key: TS14.event.key, value: JSON.stringify(TS14.event.value), headers }] });
    await producer.disconnect();
  }

  /** Reads `count` records of the output topic from the beginning. */
  async consume(count: number): Promise<{ key: string; value: unknown; headers: Record<string, string> }[]> {
    const consumer = this.kafka.consumer({ groupId: `${this.name}-verify` });
    const out: { key: string; value: unknown; headers: Record<string, string> }[] = [];
    await consumer.connect();
    await consumer.subscribe({ topic: `${this.name}-out`, fromBeginning: true });
    const done = new Promise<void>((resolve) => {
      void consumer.run({
        eachMessage: async ({ message }) => {
          out.push({ key: message.key?.toString() ?? '', value: JSON.parse(message.value?.toString() ?? 'null'), headers: Ts14Kafka.text(message.headers) });
          if (out.length >= count) resolve();
        },
      });
    });
    await Promise.race([done, new Promise((r) => setTimeout(r, TS14.kafkaMs))]);
    await consumer.disconnect();
    return out;
  }

  async cleanup(routeId: string | null, connectionId: string | null): Promise<void> {
    if (routeId) await this.archive(routeId);
    const admin = this.kafka.admin();
    await admin.connect();
    await admin.deleteTopics({ topics: [`${this.name}-in`, `${this.name}-out`] }).catch(() => undefined);
    for (const group of [`${this.name}-verify`, `router.${this.name}`]) await this.deleteGroup(admin, group);
    await admin.disconnect();
    if (connectionId) await this.page.request.delete(`/api/v1/connections/${connectionId}`, { headers: await this.session.headers() });
  }

  private async deleteGroup(admin: ReturnType<InstanceType<typeof Kafka>['admin']>, group: string): Promise<void> {
    for (let attempt = 0; attempt < TS14.groupDeleteAttempts; attempt++) {
      const deleted = await admin.deleteGroups([group]).then(() => true, () => false);
      const known = (await admin.listGroups()).groups.some((g) => g.groupId === group);
      if (deleted || !known) return;
      await new Promise((r) => setTimeout(r, TS14.groupDeleteWaitMs));
    }
  }

  private async archive(routeId: string): Promise<void> {
    const etag = async () => (await this.page.request.get(`/api/v1/routes/${routeId}`)).headers()['etag'];
    await this.page.request.post(`/api/v1/routes/${routeId}/pause`, { data: {}, headers: await this.session.headers(await etag()) });
    await this.page.request.delete(`/api/v1/routes/${routeId}`, { headers: await this.session.headers(await etag()) });
  }

  private static text(headers: IHeaders | undefined): Record<string, string> {
    return Object.fromEntries(Object.entries(headers ?? {}).map(([k, v]) => [k, String(Array.isArray(v) ? v[0] : v)]));
  }
}

test.describe('transform studio TS-14 structure + Kafka recipes', () => {
  test.setTimeout(TS14.timeoutMs);
  test.use({ viewport: TS14.viewport });

  test('each structure and header recipe form gives the preview service output and a before → after example', async ({ page }) => {
    const studio = new Ts14Session(page);
    await studio.login();
    const session = await studio.uploadSession();
    for (const recipe of recipes) {
      await test.step(recipe.label, async () => {
        await studio.openStudio(`/routes/new?step=transform&session=${session}&sample=0`);
        await studio.addStep(recipe.label);
        await recipe.fill(studio);
        await studio.previewSettled();
        await recipe.expect(studio, page);
      });
    }
  });

  test('a dev route with split array + wrap + header steps built in the Studio delivers the transformed events on Kafka', async ({ page }) => {
    const studio = new Ts14Session(page);
    await studio.login();
    const kafka = new Ts14Kafka(page, studio, `${TS14.prefix}${Date.now()}`);
    let connectionId: string | null = null;
    let routeId: string | null = null;
    try {
      connectionId = await kafka.createConnection();
      routeId = await kafka.createRoute(connectionId);
      const session = await studio.uploadSession();
      await studio.openStudio(`/routes/${routeId}/edit?step=transform&session=${session}&sample=0`);

      await studio.addStep('Split array');
      await studio.fill('Item field', 'item');
      await studio.addStep('Copy payload to header');
      await studio.pick('Field', 'orderId');
      await studio.fill('Header', 'order-id');
      await studio.addStep('Remove header');
      await studio.fill('Header', 'x-internal');
      await studio.addStep('Rename header');
      await studio.fill('Header', 'traceId');
      await studio.fill('New header', 'trace-id');
      await studio.addStep('Wrap');
      await studio.fill('Wrap in', 'payload');
      await studio.previewSettled();
      await expect(studio.outputHeaders).toContainText('order-id=o-1');
      await expect(studio.outputHeaders).toContainText('trace-id=t-1');
      await expect(studio.outputHeaders).not.toContainText('x-internal');
      await expect(studio.output).toContainText('payload');

      await page.getByRole('button', { name: 'Save draft' }).click();
      await expect
        .poll(async () => ((await (await page.request.get(`/api/v1/routes/${routeId}`)).json()).draft?.branches?.[0]?.transform?.steps ?? []).map((s: { kind: string }) => s.kind))
        .toEqual(['split_array', 'header_copy_from_payload', 'header_remove', 'header_rename', 'wrap']);

      await page.getByRole('button', { name: /Review & deploy/ }).click();
      await expect(page.getByText('Configuration is valid')).toBeVisible({ timeout: 20_000 });
      await page.getByRole('button', { name: 'Deploy route' }).click();
      await page.getByPlaceholder(/Change note/).fill('TS-14 e2e: split array + wrap + header ops');
      await page.getByRole('button', { name: 'Deploy', exact: true }).click();
      await page.waitForURL(/\/routes\/[0-9a-f-]+\?tab=overview/);

      await kafka.produce();
      const out = await kafka.consume(2);
      expect(out.map((m) => m.value)).toEqual([
        { payload: { ...TS14.event.value, item: { sku: 'A', qty: 1 } } },
        { payload: { ...TS14.event.value, item: { sku: 'B', qty: 2 } } },
      ]);
      for (const m of out) {
        expect(m.key).toBe(TS14.event.key);
        expect(m.headers).toMatchObject({ 'order-id': 'o-1', 'trace-id': 't-1' });
        expect(m.headers).not.toHaveProperty('x-internal');
        expect(m.headers).not.toHaveProperty('traceId');
      }
    } finally {
      await kafka.cleanup(routeId, connectionId);
    }
  });
});
