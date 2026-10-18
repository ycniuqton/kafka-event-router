import type { Services } from '../api';
import type { Alert } from '../domain';
import type { AppStore } from './AppStore';
import type { ConnectionActions } from './ConnectionActions';
import { PushEventRouter, type PushEffects } from './PushEventRouter';
import type { RouteActions } from './RouteActions';
import type { ToastController } from './ToastController';

/** In-app notifications (e.g. finished dead-letter jobs) have no server state; server alerts always do. */
export class AlertOrigin {
  static isLocal(alert: Alert): boolean {
    return alert.state === undefined;
  }
}

/** Bell feed, acknowledgements and the `WS /events` subscription that keeps shared data fresh. */
export class MonitoringActions implements PushEffects {
  private readonly store: AppStore;
  private readonly services: Services;
  private readonly toast: ToastController;
  private readonly routes: RouteActions;
  private readonly connections: ConnectionActions;
  private stopPush: (() => void) | null = null;

  constructor(store: AppStore, services: Services, toast: ToastController, routes: RouteActions, connections: ConnectionActions) {
    this.store = store;
    this.services = services;
    this.toast = toast;
    this.routes = routes;
    this.connections = connections;
  }

  /** Reloads the bell (alerts + unread count); failures keep the last state. */
  async refreshAlerts(): Promise<void> {
    try {
      const feed = await this.services.alerts.feed();
      const served = new Set(feed.items.map((a) => a.id));
      const local = this.store.getState().alerts.filter((a) => AlertOrigin.isLocal(a) && !served.has(a.id));
      this.store.setState({ alerts: [...local, ...feed.items], alertsUnread: feed.unread + local.filter((a) => !a.acked).length });
    } catch {
      return;
    }
  }

  /** Acknowledges one alert: server alerts via the API, in-app notifications locally. */
  async ack(id: string): Promise<void> {
    const alert = this.store.getState().alerts.find((a) => a.id === id);
    if (!alert || alert.acked) return;
    if (AlertOrigin.isLocal(alert)) this.store.setState((s) => ({ alerts: s.alerts.map((a) => (a.id === id ? { ...a, acked: true } : a)) }));
    else await this.services.alerts.ack(id);
    await this.refreshAlerts();
  }

  /** Acknowledges every unread alert in the bell. */
  async ackAll(): Promise<void> {
    const unread = this.store.getState().alerts.filter((a) => !a.acked);
    await Promise.all(unread.map((a) => (AlertOrigin.isLocal(a) ? Promise.resolve() : this.services.alerts.ack(a.id))));
    this.store.setState((s) => ({ alerts: s.alerts.map((a) => (AlertOrigin.isLocal(a) ? { ...a, acked: true } : a)) }));
    await this.refreshAlerts();
  }

  /** Subscribes once to push frames for the whole session. */
  startPush(): void {
    if (this.stopPush) return;
    const router = new PushEventRouter(this);
    const offState = this.services.push.onState((pushState) => this.store.setState({ pushState }));
    const offFrames = this.services.push.subscribe((event) => router.apply(event));
    this.store.setState({ pushState: this.services.push.state() });
    this.stopPush = () => {
      offState();
      offFrames();
    };
  }

  stop(): void {
    this.stopPush?.();
    this.stopPush = null;
  }

  refreshRoute(routeId: string): void {
    void this.routes.refresh(routeId).catch(() => undefined);
  }

  refreshConnections(): void {
    void this.connections.refresh().catch(() => undefined);
  }

  announce(message: string): void {
    this.toast.show(message);
  }

  touch(): void {
    this.store.setState((s) => ({ pushSeq: s.pushSeq + 1 }));
  }
}
