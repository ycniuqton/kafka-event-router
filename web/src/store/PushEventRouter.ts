import { PushEventType, type PushEvent } from '../domain';

/** What applying push frames may do to the app (implemented by MonitoringActions, faked in tests). */
export interface PushEffects {
  refreshRoute(routeId: string): void;
  refreshConnections(): void;
  refreshAlerts(): void;
  announce(message: string): void;
  /** Marks shared data as changed so pages re-fetch summaries (dashboard cards, attention). */
  touch(): void;
}

/** Applies one frame type to the app. */
export abstract class PushFrameReducer {
  abstract readonly type: PushEventType;
  abstract apply(event: PushEvent, effects: PushEffects): void;

  protected text(event: PushEvent, key: string): string {
    const value = event.payload[key];
    return typeof value === 'string' ? value : '';
  }
}

/** Route status changed → re-read the route (list badge, detail header). */
class RouteStateReducer extends PushFrameReducer {
  readonly type = PushEventType.RouteStateChanged;
  apply(event: PushEvent, effects: PushEffects): void {
    const id = this.text(event, 'routeId');
    if (id) effects.refreshRoute(id);
    effects.touch();
  }
}

/** Deployment status changed → re-read the route (version, rollout state). */
class DeploymentReducer extends PushFrameReducer {
  readonly type = PushEventType.DeploymentStatus;
  apply(event: PushEvent, effects: PushEffects): void {
    const id = this.text(event, 'routeId');
    if (id) effects.refreshRoute(id);
    effects.touch();
  }
}

/** Connection health changed → reload connection statuses. */
class ConnectionHealthReducer extends PushFrameReducer {
  readonly type = PushEventType.ConnectionHealth;
  apply(_event: PushEvent, effects: PushEffects): void {
    effects.refreshConnections();
    effects.touch();
  }
}

/** Alert fired → bell + toast. */
class AlertFiredReducer extends PushFrameReducer {
  readonly type = PushEventType.AlertFired;
  apply(event: PushEvent, effects: PushEffects): void {
    effects.refreshAlerts();
    effects.announce(`Alert firing: ${[this.text(event, 'routeName'), this.text(event, 'ruleName')].filter(Boolean).join(' · ')}`);
    effects.touch();
  }
}

/** Alert resolved → bell. */
class AlertResolvedReducer extends PushFrameReducer {
  readonly type = PushEventType.AlertResolved;
  apply(_event: PushEvent, effects: PushEffects): void {
    effects.refreshAlerts();
    effects.touch();
  }
}

/** The client fell behind and frames were dropped → refetch everything push keeps fresh. */
class ResyncReducer extends PushFrameReducer {
  readonly type = PushEventType.Resync;
  apply(_event: PushEvent, effects: PushEffects): void {
    effects.refreshAlerts();
    effects.refreshConnections();
    effects.touch();
  }
}

/** Routes `WS /events` frames to their reducers (unknown types are ignored). */
export class PushEventRouter {
  private readonly reducers = new Map<string, PushFrameReducer>();
  private readonly effects: PushEffects;

  constructor(effects: PushEffects, reducers: PushFrameReducer[] = PushEventRouter.defaults()) {
    this.effects = effects;
    reducers.forEach((r) => this.reducers.set(r.type, r));
  }

  static defaults(): PushFrameReducer[] {
    return [new RouteStateReducer(), new DeploymentReducer(), new ConnectionHealthReducer(), new AlertFiredReducer(), new AlertResolvedReducer(), new ResyncReducer()];
  }

  apply(event: PushEvent): void {
    this.reducers.get(event.type)?.apply(event, this.effects);
  }
}
