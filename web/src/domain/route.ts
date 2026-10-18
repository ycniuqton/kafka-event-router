import { Tone } from './tone';

/** Lifecycle state of a route (spec §2: Draft → Running ↔ Paused, plus Error and Archived). */
export const RouteStatus = {
  Running: 'running',
  Paused: 'paused',
  Error: 'error',
  Draft: 'draft',
  Archived: 'archived',
} as const;
export type RouteStatus = (typeof RouteStatus)[keyof typeof RouteStatus];

/** Kind of transform a route applies, summarised for lists. */
export const TransformMode = { None: 'none', Recipe: 'recipe', Js: 'js' } as const;
export type TransformMode = (typeof TransformMode)[keyof typeof TransformMode];

export type RouteEndpoint = { connectionId: string; connectionName?: string; topics: string[] };
export type RouteTarget = { connectionId: string; connectionName?: string; topic: string };
/** Live numbers of a route; `unavailable` when the metrics backend has none (shown as "—"). */
export type RouteMetrics = { eventsPerSec: number; matchPct: number; lag: number; sparkline?: number[]; unavailable?: boolean };

/** A route as listed in the UI (design `ROUTES`). */
export type Route = {
  id: string;
  name: string;
  status: RouteStatus;
  owner: string;
  source: RouteEndpoint;
  destination: RouteTarget;
  filterSummary: string;
  transformMode: TransformMode;
  metrics: RouteMetrics;
  updatedAt: string;
  version: number;
  tags?: string[];
  updatedBy?: string;
  hasUndeployedDraft?: boolean;
};

type StatusPresentation = { label: string; tone: Tone };

/** Label and tone for each route status: green running, grey paused, red error, blue draft. */
export const RouteStatusMeta: Record<RouteStatus, StatusPresentation> = {
  [RouteStatus.Running]: { label: 'Running', tone: Tone.Ok },
  [RouteStatus.Paused]: { label: 'Paused', tone: Tone.Off },
  [RouteStatus.Error]: { label: 'Error', tone: Tone.Err },
  [RouteStatus.Draft]: { label: 'Draft', tone: Tone.Info },
  [RouteStatus.Archived]: { label: 'Archived', tone: Tone.Muted },
};

/** Short and long labels plus tone for the transform mode. */
export const TransformModeMeta: Record<TransformMode, { short: string; long: string; tone: Tone }> = {
  [TransformMode.None]: { short: '—', long: 'No transform', tone: Tone.Muted },
  [TransformMode.Recipe]: { short: 'recipe', long: 'Recipe steps', tone: Tone.TextSecondary },
  [TransformMode.Js]: { short: 'JS', long: 'JS + recipes', tone: Tone.Code },
};

/** Business rules about which actions a route's status allows. */
export class RouteRules {
  static canPause(status: RouteStatus): boolean {
    return status === RouteStatus.Running || status === RouteStatus.Paused;
  }

  static isLive(status: RouteStatus): boolean {
    return status === RouteStatus.Running;
  }

  static pauseLabel(status: RouteStatus): string {
    return status === RouteStatus.Paused ? 'Resume' : 'Pause';
  }
}

/** Display text of a route's endpoints: connection name when known (REST), else its id (mock data). */
export class RouteEndpointText {
  static readonly topicJoiner = ', ';
  static readonly separator = ' / ';

  static sourceConnection(route: Route): string {
    return route.source.connectionName || route.source.connectionId;
  }

  static destinationConnection(route: Route): string {
    return route.destination.connectionName || route.destination.connectionId;
  }

  static source(route: Route): string {
    return `${RouteEndpointText.sourceConnection(route)}${RouteEndpointText.separator}${route.source.topics.join(RouteEndpointText.topicJoiner)}`;
  }

  static destination(route: Route): string {
    return `${RouteEndpointText.destinationConnection(route)}${RouteEndpointText.separator}${route.destination.topic}`;
  }
}
