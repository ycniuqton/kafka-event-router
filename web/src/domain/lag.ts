import type { LagThresholds } from './metrics';
import { Tone } from './tone';

/** Consumer-lag thresholds from the design, per route and for the dashboard total (server values override). */
export const LagThreshold = {
  RouteWarn: 2_000,
  RouteError: 20_000,
  TotalWarn: 10_000,
  TotalError: 40_000,
} as const;

/** Classifies lag values into status tones against the active (server or design) thresholds. */
export class LagHealth {
  private static active: LagThresholds = {
    routeLagWarn: LagThreshold.RouteWarn,
    routeLagError: LagThreshold.RouteError,
    totalLagWarn: LagThreshold.TotalWarn,
    totalLagError: LagThreshold.TotalError,
  };

  /** Adopts the thresholds the server returned with the dashboard summary. */
  static configure(thresholds: LagThresholds): void {
    LagHealth.active = { ...thresholds };
  }

  static thresholds(): LagThresholds {
    return LagHealth.active;
  }

  static routeTone(lag: number): Tone {
    if (lag > LagHealth.active.routeLagError) return Tone.Err;
    return lag > LagHealth.active.routeLagWarn ? Tone.Warn : Tone.TextSecondary;
  }

  static totalTone(lag: number): Tone {
    if (lag > LagHealth.active.totalLagError) return Tone.Err;
    return lag > LagHealth.active.totalLagWarn ? Tone.Warn : Tone.Ok;
  }

  static totalNote(lag: number): string {
    const limit = LagHealth.active.totalLagError;
    return lag > limit ? `Above ${LagHealth.compact(limit)} threshold` : 'Within threshold';
  }

  private static compact(n: number): string {
    const thousand = 1_000;
    return n >= thousand && n % thousand === 0 ? `${n / thousand}k` : String(n);
  }
}
