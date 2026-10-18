import { Format } from '../../../components/ui';
import { RouteStatus, type Route } from '../../../domain';
import { RouteDetailModel } from './RouteDetailModel';

export type InOutPoint = { inPct: number; outPct: number };
export type PartitionLag = { partition: string; lag: number; heightPct: number; label: string };
export type LatencyPoint = { p50: number; p95: number };

/** Deterministic demo time series for the overview charts (design `ioBars`, `lagBars`). */
export class MetricSeries {
  static readonly points = 36;
  static readonly partitions = 6;
  static readonly failureStart = 22;
  static readonly idlePct = 2;
  static readonly failedOutPct = 1;
  static readonly minOutShare = 0.06;
  static readonly partitionPrefix = 'P';

  static inOut(route: Route, rangeSeed: number): InOutPoint[] {
    return Array.from({ length: MetricSeries.points }, (_, i) => MetricSeries.inOutAt(route, i, rangeSeed));
  }

  static partitionLag(route: Route): PartitionLag[] {
    const lag = route.metrics.lag;
    const even = lag / MetricSeries.partitions;
    return Array.from({ length: MetricSeries.partitions }, (_, i) => {
      const value = Math.round(even * (0.6 + 0.8 * Math.abs(Math.sin(i + lag))));
      const heightPct = lag ? Math.max(4, Math.min(100, (value / even) * 60)) : 3;
      return { partition: `${MetricSeries.partitionPrefix}${i}`, lag: value, heightPct, label: Format.number(value) };
    });
  }

  static latency(route: Route, rangeSeed: number): LatencyPoint[] {
    const base = new RouteDetailModel(route).latency;
    return Array.from({ length: MetricSeries.points }, (_, i) => {
      const wobble = Math.sin(i * 0.7 + rangeSeed) * 0.18 + Math.sin(i * 0.23) * 0.1;
      return { p50: Math.round(base.p50 * (1 + wobble)), p95: Math.round(base.p95 * (1 + wobble * 1.6)) };
    });
  }

  static errors(route: Route, rangeSeed: number): number[] {
    const perMin = new RouteDetailModel(route).errorsPerMin;
    return Array.from({ length: MetricSeries.points }, (_, i) =>
      perMin && i >= MetricSeries.failureStart ? Number((perMin * (0.7 + 0.5 * Math.abs(Math.sin(i + rangeSeed)))).toFixed(1)) : 0,
    );
  }

  private static inOutAt(route: Route, i: number, rangeSeed: number): InOutPoint {
    const failed = route.status === RouteStatus.Error;
    const inPct = MetricSeries.inPctAt(route, i, rangeSeed);
    const share = Math.max(MetricSeries.minOutShare, route.metrics.matchPct / RouteDetailModel.percent);
    const outPct = failed && i >= MetricSeries.failureStart ? MetricSeries.failedOutPct : inPct * share;
    return { inPct: Math.round(inPct), outPct: Math.round(outPct) };
  }

  private static inPctAt(route: Route, i: number, rangeSeed: number): number {
    const seed = route.metrics.eventsPerSec + rangeSeed;
    if (route.status === RouteStatus.Running) return 40 + 30 * Math.abs(Math.sin(i * 0.4 + seed)) + 12 * Math.sin(i * 1.3);
    if (route.status === RouteStatus.Error && i < MetricSeries.failureStart) return 50 + 20 * Math.sin(i * 0.5);
    return MetricSeries.idlePct;
  }
}
