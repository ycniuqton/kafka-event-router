import type { LagThresholds } from './metrics';
import type { Tone } from './tone';

/** Route counts of the dashboard routes card (server summary). */
export type RouteStateCounts = { running: number; paused: number; error: number; draft: number };

/**
 * Aggregate numbers on the dashboard that are not derived from the routes list.
 * The REST source adds the server summary fields; `metricsAvailable: false` means the metrics store is down.
 */
export type DashboardStats = {
  eventsLastHour: number;
  sparkSeed: number;
  spark?: number[];
  totalLag?: { value: number; tone: Tone } | null;
  deadLetters24h?: number | null;
  routes?: RouteStateCounts;
  thresholds?: LagThresholds;
  metricsAvailable?: boolean;
};
