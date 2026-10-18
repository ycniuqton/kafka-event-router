import { describe, expect, it } from 'vitest';
import { MetricsRange, type MetricPoint, type RouteOverviewMetrics } from '../../../domain';
import { OverviewSeriesView } from './OverviewSeriesView';

const points = (n: number, f: (i: number) => number): MetricPoint[] => Array.from({ length: n }, (_, i) => [1_700_000_000 + i * 30, f(i)]);

const metrics: RouteOverviewMetrics = {
  range: MetricsRange.H1,
  step: '30s',
  pipeline: [
    { branch: 'main', read: 100, passed: 80, sent: 78, dlq: 2 },
    { branch: 'eu', read: 10, passed: 5, sent: 5, dlq: 0 },
  ],
  series: {
    eventsIn: points(72, () => 10),
    eventsOut: points(72, () => 5),
    lagByPartition: { '0': points(3, (i) => i * 100), other: points(3, () => 50) },
    latencyP50: points(10, () => 0.012),
    latencyP95: points(10, () => 0.05),
    errorsPerMin: points(10, () => 0.25),
  },
};

describe('OverviewSeriesView', () => {
  it('reports an idle range as empty and a range with samples as not empty', () => {
    const idle: RouteOverviewMetrics = {
      ...metrics,
      pipeline: [],
      series: { eventsIn: [], eventsOut: [], lagByPartition: {}, latencyP50: [], latencyP95: [], errorsPerMin: [] },
    };
    expect(OverviewSeriesView.isEmpty(idle)).toBe(true);
    expect(OverviewSeriesView.isEmpty(metrics)).toBe(false);
  });

  it('sums branch rows into pipeline counts', () => {
    expect(OverviewSeriesView.counts(metrics)).toEqual({ read: 110, passed: 85, sent: 83, deadLetters: 2 });
  });

  it('scales in/out to the peak, buckets long series and converts latency to ms', () => {
    const data = OverviewSeriesView.chartData(metrics);
    expect(data.io).toHaveLength(OverviewSeriesView.buckets);
    expect(data.io[0]).toEqual({ inPct: 100, outPct: 50 });
    expect(data.latency[0]).toEqual({ p50: 12, p95: 50 });
    expect(data.errors[0]).toBe(0.3);
    expect(data.lag.map((p) => [p.partition, p.lag])).toEqual([['P0', 200], ['other', 50]]);
    expect(data.lag[0].heightPct).toBe(100);
  });
});
