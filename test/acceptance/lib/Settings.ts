import { expect } from '@playwright/test';
import type { AccApi } from './Api';

/** Workspace settings endpoint (access-control). */
export const SettingsPath = '/api/v1/workspace-settings' as const;

/** Temporarily changes workspace settings and restores the previous values afterwards. */
export class WorkspaceSettings {
  constructor(private readonly api: AccApi) {}

  async current(): Promise<{ body: any; ifMatch: string }> {
    const res = await this.api.get(SettingsPath);
    return { body: res.body, ifMatch: res.etag || `"${res.body.rowVersion}"` };
  }

  async set(patch: Record<string, unknown>): Promise<void> {
    const { ifMatch } = await this.current();
    const res = await this.api.patch(SettingsPath, patch, ifMatch);
    expect(res.ok, res.describe()).toBeTruthy();
  }

  /** Runs fn with the patch applied, then puts the touched keys back. */
  async with<T>(patch: Record<string, unknown>, fn: () => Promise<T>): Promise<T> {
    const { body } = await this.current();
    const previous = Object.fromEntries(Object.keys(patch).map((k) => [k, body[k]]));
    await this.set(patch);
    try {
      return await fn();
    } finally {
      await this.set(previous);
    }
  }
}
