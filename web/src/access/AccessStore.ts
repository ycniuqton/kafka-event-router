import { services } from '../api';
import type { Access } from '../domain';
import { ExternalStore } from '../store/ExternalStore';

/** Load state of the caller's effective access. */
export const AccessLoad = { Idle: 'idle', Loading: 'loading', Ready: 'ready', Failed: 'failed' } as const;
export type AccessLoad = (typeof AccessLoad)[keyof typeof AccessLoad];

export type AccessState = { access: Access | null; load: AccessLoad };

/**
 * Holds the caller's effective access (from `/auth/me`). Loaded on first use and reloaded with
 * `refresh()` after role changes; the server remains the authority on every call.
 */
export class AccessStore extends ExternalStore<AccessState> {
  constructor() {
    super({ access: null, load: AccessLoad.Idle });
  }

  /** Starts the first load once. */
  ensureLoaded(): void {
    if (this.getState().load === AccessLoad.Idle) void this.refresh();
  }

  async refresh(): Promise<void> {
    this.setState({ load: AccessLoad.Loading });
    try {
      this.setState({ access: await services.access.me(), load: AccessLoad.Ready });
    } catch {
      this.setState({ access: null, load: AccessLoad.Failed });
    }
  }
}

export const accessStore = new AccessStore();
