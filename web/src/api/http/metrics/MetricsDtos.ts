import type { LagThresholds, RouteOverviewMetrics, RouteStateCounts, Tone } from '../../../domain';

/** Paths of the monitoring metrics endpoints (relative to `/api/v1`). */
export const MetricsEndpoint = {
  summary: '/dashboard/summary',
  routeMetrics: (routeId: string) => `/routes/${encodeURIComponent(routeId)}/metrics`,
  flowMap: '/flow-map',
} as const;

/** Query parameter of the overview range. */
export const MetricsParam = { range: 'range' } as const;

/** `GET /dashboard/summary`; metric cards are null when the metrics store is unavailable. */
export type DashboardSummaryDto = {
  routes: RouteStateCounts;
  eventsLastHour: { total: number; spark: number[] } | null;
  totalLag: { value: number; tone: Tone } | null;
  dlq24h: { value: number } | null;
  thresholds: LagThresholds;
  metricsAvailable: boolean;
};

/** `GET /routes/{id}/metrics` (already in the domain shape). */
export type RouteMetricsDto = RouteOverviewMetrics;
