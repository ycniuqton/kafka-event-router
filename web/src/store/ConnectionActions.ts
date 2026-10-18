import type { Services } from '../api';
import {
  ConnectionDisplay,
  ConnectionInUseError,
  type Connection,
  type ConnectionDraft,
  type ConnectionRouteRef,
  type ConnectionTestOptions,
  type ConnectionTestRun,
  type TopicSettings,
} from '../domain';
import type { AppStore } from './AppStore';
import type { ToastController } from './ToastController';

/** Toast texts of connection actions. */
export const ConnectionToast = {
  deleted: 'Connection deleted',
  updated: 'Connection updated',
  inUsePrefix: 'Still used by: ',
  listSeparator: ', ',
  topicCreated: (name: string, partitions: number) => `Topic ${name} created · ${partitions} partitions`,
} as const;

/** Connection actions: test (live progress in the store), create, edit, delete, create topic. */
export class ConnectionActions {
  private readonly store: AppStore;
  private readonly services: Services;
  private readonly toast: ToastController;

  constructor(store: AppStore, services: Services, toast: ToastController) {
    this.store = store;
    this.services = services;
    this.toast = toast;
  }

  /** Runs a test; progress is published in `connectionTests[id]`; statuses refresh when it ends. */
  async test(id: string, options?: ConnectionTestOptions): Promise<ConnectionTestRun> {
    const run = await this.track(id, (publish) => this.services.connections.test(id, publish, options));
    await this.refresh().catch(() => undefined);
    return run;
  }

  /** Tests the panel's unsaved draft; progress is published under `ConnectionDisplay.draftTestKey`. */
  /** Rejects (after publishing a failed run) so the panel can highlight the fields of a 422. */
  async testDraft(draft: ConnectionDraft, connectionId: string | null, options?: ConnectionTestOptions): Promise<ConnectionTestRun> {
    const key = ConnectionDisplay.draftTestKey(connectionId);
    const run = await this.track(key, (publish) => this.services.connections.testDraft(draft, connectionId, publish, options), true);
    return run;
  }

  async create(draft: ConnectionDraft): Promise<Connection> {
    const created = await this.services.connections.create(draft);
    this.store.setState((s) => ({ connections: [...s.connections, created] }));
    void this.test(created.id);
    return created;
  }

  /** Saves an edit, then re-tests the connection. */
  async update(id: string, draft: ConnectionDraft): Promise<Connection> {
    const updated = await this.services.connections.update(id, draft);
    this.replace(updated);
    this.toast.show(ConnectionToast.updated);
    this.clearTest(id);
    void this.test(id);
    return updated;
  }

  /** Deletes an unused connection; returns false (with a toast naming the routes) when routes still use it. */
  async remove(id: string): Promise<boolean> {
    try {
      await this.services.connections.remove(id);
    } catch (error) {
      if (!(error instanceof ConnectionInUseError)) throw error;
      this.toast.show(ConnectionToast.inUsePrefix + error.routes.map((r) => r.name).join(ConnectionToast.listSeparator));
      return false;
    }
    this.store.setState((s) => ({ connections: s.connections.filter((c) => c.id !== id) }));
    this.toast.show(ConnectionToast.deleted);
    return true;
  }

  async createTopic(id: string, name: string, settings?: TopicSettings): Promise<void> {
    const topic = await this.services.connections.createTopic(id, name, settings);
    this.store.setState((s) => ({
      connections: s.connections.map((c) => (c.id === id ? { ...c, topics: [...c.topics, topic] } : c)),
    }));
    this.toast.show(ConnectionToast.topicCreated(name, topic.partitions));
  }

  clearTest(id: string): void {
    this.store.setState((s) => {
      const rest = { ...s.connectionTests };
      delete rest[id];
      return { connectionTests: rest };
    });
  }

  /** Routes using a connection: the server's count/list when known, else the routes in the store. */
  usedBy(id: string): ConnectionRouteRef[] {
    const state = this.store.getState();
    const known = state.connections.find((c) => c.id === id)?.usedBy?.routes;
    if (known) return known;
    return state.routes.filter((r) => r.source.connectionId === id || r.destination.connectionId === id).map((r) => ({ id: r.id, name: r.name }));
  }

  /** Reloads the list (statuses, used-by counts). */
  async refresh(): Promise<void> {
    const connections = await this.services.connections.list();
    this.store.setState({ connections });
  }

  /** Runs a test, publishing snapshots; a failure becomes a finished run carrying the error text. */
  private async track(
    key: string,
    run: (publish: (r: ConnectionTestRun) => void) => Promise<ConnectionTestRun>,
    rethrow = false,
  ): Promise<ConnectionTestRun> {
    try {
      return await run((r) => this.publish(key, r));
    } catch (error) {
      const last = this.store.getState().connectionTests[key];
      const failed: ConnectionTestRun = { connectionId: key, steps: last?.steps ?? [], finished: true, ok: false, error: (error as Error).message };
      this.publish(key, failed);
      if (rethrow) throw error;
      return failed;
    }
  }

  private publish(key: string, run: ConnectionTestRun): void {
    this.store.setState((s) => ({ connectionTests: { ...s.connectionTests, [key]: run } }));
  }

  private replace(connection: Connection): void {
    this.store.setState((s) => ({ connections: s.connections.map((c) => (c.id === connection.id ? { ...c, ...connection } : c)) }));
  }
}
