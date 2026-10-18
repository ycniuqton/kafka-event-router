import { expect, type Locator, type Page } from '@playwright/test';

/** UI timings (preview service round trips, worker convergence). */
export const UiTiming = { previewMs: 30_000, pageMs: 30_000 } as const;

/** Small page-object helpers shared by the UI specs. */
export class Ui {
  constructor(readonly page: Page) {}

  get main(): Locator {
    return this.page.getByRole('main');
  }

  /** Picks the `<option>` whose label contains text in the select (first select of the page when label is null). */
  async pick(label: string | null, text: string, index = 0): Promise<void> {
    const select = label ? this.page.getByLabel(label).nth(index) : this.page.locator('select').first();
    const value = await select.locator('option', { hasText: text }).first().getAttribute('value');
    await select.selectOption(value ?? '');
  }

  /** Builder source step: name, connection and input topic. */
  async builderSource(name: string, connection: string, topic: string): Promise<void> {
    await this.page.getByRole('textbox', { name: 'Route name' }).fill(name);
    await this.pick(null, connection);
    await this.page.getByRole('option', { name: new RegExp(topic) }).click();
  }

  async builderDestination(topic: string): Promise<void> {
    await this.page.getByRole('button', { name: /^Destination/ }).first().click();
    await this.page.locator('input[list="destination-topics"]').fill(topic);
  }

  /** Save draft → Review & deploy → Deploy with a note; returns the route id. */
  async saveAndDeploy(note: string): Promise<string> {
    await this.page.getByRole('button', { name: 'Save draft' }).click();
    await this.page.waitForURL(/\/routes\/[0-9a-f-]+\/edit/);
    const id = new URL(this.page.url()).pathname.split('/')[2];
    await this.page.getByRole('button', { name: /Review & deploy/ }).click();
    await expect(this.page.getByText('Configuration is valid')).toBeVisible({ timeout: UiTiming.previewMs });
    await this.page.getByRole('button', { name: 'Deploy route' }).click();
    await this.page.getByPlaceholder(/Change note/).fill(note);
    await this.page.getByRole('button', { name: 'Deploy', exact: true }).click();
    await this.page.waitForURL(/\/routes\/[0-9a-f-]+\?tab=overview/);
    return id;
  }

  /** Transform Studio: + Add step → search → pick the recipe. */
  async addStep(label: string): Promise<void> {
    await this.page.getByRole('button', { name: '+ Add step' }).click();
    await this.page.getByLabel('Search steps').fill(label);
    await this.page.getByRole('button', { name: label, exact: true }).click();
  }

  async uploadSamples(ndjson: string): Promise<void> {
    await this.page.getByRole('radio', { name: 'Upload' }).click();
    await this.page.locator('input[type="file"]').setInputFiles({ name: 'events.ndjson', mimeType: 'application/x-ndjson', buffer: Buffer.from(ndjson) });
  }
}
