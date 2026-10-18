import { describe, expect, it } from 'vitest';
import { SparkSeries } from '../../../components/ui/SparkSeries';
import { LagHealth, type DashboardStats } from '../../../domain';
import { DashboardFreshness, DashboardSummary } from './DashboardSummary';

const thresholds = { routeLagWarn: 2_000, routeLagError: 20_000, totalLagWarn: 10_000, totalLagError: 40_000 };

const stats = (over: Partial<DashboardStats>): DashboardStats => ({ eventsLastHour: 0, sparkSeed: 0, ...over });

describe('DashboardSummary', () => {
  it('prefers the server route counts and lag over the routes list', () => {
    LagHealth.configure(thresholds);
    const summary = DashboardSummary.of([], stats({ routes: { running: 4, paused: 2, error: 1, draft: 0 }, totalLag: { value: 12_000, tone: 'warn' } }));
    expect(summary).toMatchObject({ running: 4, paused: 2, errors: 1, totalLag: 12_000, metricsAvailable: true });
    expect(summary.lagTone).toBe(LagHealth.totalTone(12_000));
  });

  it('flags degraded mode when the metrics store is unavailable', () => {
    expect(DashboardSummary.of([], stats({ metricsAvailable: false, totalLag: null })).metricsAvailable).toBe(false);
  });
});

describe('DashboardFreshness', () => {
  it('formats the seconds since the last successful poll', () => {
    expect(DashboardFreshness.subtitle(null, 0)).toBe(DashboardFreshness.waiting);
    expect(DashboardFreshness.subtitle(10_000, 14_400)).toBe('Updated 4 s ago · live');
  });
});

describe('SparkSeries.fromValues', () => {
  it('scales a real series to its peak and keeps an idle series flat', () => {
    expect(SparkSeries.fromValues([0, 5, 10])).toEqual([SparkSeries.minHeight, 50, 100]);
    expect(SparkSeries.fromValues([0, 0])).toEqual([SparkSeries.flatHeight, SparkSeries.flatHeight]);
  });
});
