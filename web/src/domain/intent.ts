import type { BuilderStep } from './builder';

/** Tabs of the route detail page (URL `?tab=`). */
export const RouteDetailTab = { Overview: 'overview', Tail: 'tail', Dead: 'dead', History: 'history', Settings: 'settings' } as const;
export type RouteDetailTab = (typeof RouteDetailTab)[keyof typeof RouteDetailTab];

export const IntentType = {
  RouteDetail: 'routeDetail',
  RouteEdit: 'routeEdit',
  TestConnection: 'testConnection',
  TogglePause: 'togglePause',
} as const;

/**
 * What should happen when the user acts on an alert or attention item.
 * Data stays serialisable; the app resolves intents to navigation or store actions.
 */
export type ActionIntent =
  | { type: typeof IntentType.RouteDetail; routeId: string; tab: RouteDetailTab }
  | { type: typeof IntentType.RouteEdit; routeId: string; step: BuilderStep }
  | { type: typeof IntentType.TestConnection; connectionId: string }
  | { type: typeof IntentType.TogglePause; routeId: string };
