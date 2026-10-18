import type { Services } from '../api';
import type { AppStore } from './AppStore';
import { ConnectionActions } from './ConnectionActions';
import { DeadLetterActions } from './DeadLetterActions';
import { MonitoringActions } from './MonitoringActions';
import { RouteActions } from './RouteActions';
import { ToastController } from './ToastController';

/** Entry point for every state change: bootstrap, shell UI and per-domain action groups. */
export class AppActions {
  readonly toast: ToastController;
  readonly routes: RouteActions;
  readonly connections: ConnectionActions;
  readonly deadLetters: DeadLetterActions;
  readonly monitoring: MonitoringActions;
  private readonly store: AppStore;
  private readonly services: Services;

  constructor(store: AppStore, services: Services) {
    this.store = store;
    this.services = services;
    this.toast = new ToastController(store);
    this.routes = new RouteActions(store, services, this.toast);
    this.connections = new ConnectionActions(store, services, this.toast);
    this.deadLetters = new DeadLetterActions(store, services, this.toast);
    this.monitoring = new MonitoringActions(store, services, this.toast, this.routes, this.connections);
  }

  /** Loads the shared collections once at startup. */
  async bootstrap(): Promise<void> {
    if (this.store.getState().ready) return;
    const s = this.services;
    const [routes, connections, activity, alerts, currentUser] = await Promise.all([
      s.routes.list(),
      s.connections.list(),
      s.activity.list(),
      s.alerts.list(),
      s.members.current(),
    ]);
    this.store.setState({ ready: true, routes, connections, activity, alerts, currentUser });
    void this.deadLetters.refreshSummary();
    void this.monitoring.refreshAlerts();
    this.monitoring.startPush();
  }

  openNewRouteChooser(): void {
    this.store.setState({ newRouteChooserOpen: true });
  }

  closeNewRouteChooser(): void {
    this.store.setState({ newRouteChooserOpen: false });
  }
}
