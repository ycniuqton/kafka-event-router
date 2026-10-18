/** Frame types of `WS /events` (server `PushEventType`). */
export const PushEventType = {
  RouteStateChanged: 'route.state_changed',
  DeploymentStatus: 'deployment.status_changed',
  ConnectionHealth: 'connection.health_changed',
  AlertFired: 'alert.fired',
  AlertResolved: 'alert.resolved',
  Hello: 'hello',
  Resync: 'resync',
  RouteReplayChanged: 'route.replay_changed',
} as const;
export type PushEventType = (typeof PushEventType)[keyof typeof PushEventType];

/** One push frame. */
export type PushEvent = { type: PushEventType; at: string; payload: Record<string, unknown> };

/** Connection state of the push stream (drives the dashboard's fallback polling). */
export const PushState = { Connecting: 'connecting', Live: 'live', Offline: 'offline' } as const;
export type PushState = (typeof PushState)[keyof typeof PushState];
