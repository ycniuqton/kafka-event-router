import { LagHealth, RouteStatus, type DashboardStats, type Route, type RouteStateCounts, type Tone } from '../../../domain';

/** Numbers behind the four health cards: server summary first, the routes list only before it arrives. */
export type HealthSummary = {
  running: number;
  paused: number;
  errors: number;
  totalLag: number;
  lagTone: Tone;
  lagNote: string;
  metricsAvailable: boolean;
};

/** Builds the dashboard health summary (design `dRunning…dLagNote`); server counts and lag win when present. */
export class DashboardSummary {
  static of(routes: Route[], stats?: DashboardStats): HealthSummary {
    const counts = stats?.routes ?? DashboardSummary.countAll(routes);
    const totalLag = stats?.totalLag?.value ?? routes.reduce((sum, r) => sum + r.metrics.lag, 0);
    return {
      running: counts.running,
      paused: counts.paused,
      errors: counts.error,
      totalLag,
      lagTone: LagHealth.totalTone(totalLag),
      lagNote: LagHealth.totalNote(totalLag),
      metricsAvailable: stats?.metricsAvailable !== false,
    };
  }

  private static countAll(routes: Route[]): RouteStateCounts {
    const count = (status: RouteStatus) => routes.filter((r) => r.status === status).length;
    return { running: count(RouteStatus.Running), paused: count(RouteStatus.Paused), error: count(RouteStatus.Error), draft: count(RouteStatus.Draft) };
  }
}

/** Feed sizes on the dashboard (spec §4: the last 10 changes). */
export const DashboardLimits = { recentActivity: 10 } as const;

/** Shown on metric cards when the metrics store is down (counts from Postgres still render). */
export const MetricsUnavailable = { value: 'Unavailable', footer: 'Metrics store unreachable · retrying' } as const;

/** "Updated 4 s ago · live" header line of the polled dashboard. */
export class DashboardFreshness {
  static readonly msPerSecond = 1_000;
  static readonly tickMs = 1_000;
  static readonly waiting = 'Loading…';

  /** Resolves `load` together with the clock time it arrived. */
  static async stamp<T>(load: Promise<T>, clock: { now(): Date }): Promise<{ value: T; at: number }> {
    const value = await load;
    return { value, at: clock.now().getTime() };
  }

  /** "· live" while the push stream is connected, "· polling" while the dashboard falls back to polling. */
  static subtitle(updatedAt: number | null, now: number, live = true): string {
    if (updatedAt === null) return DashboardFreshness.waiting;
    const seconds = Math.max(0, Math.round((now - updatedAt) / DashboardFreshness.msPerSecond));
    return `Updated ${seconds} s ago · ${live ? 'live' : 'polling'}`;
  }
}
