import { Format } from '../../../components/ui/Format';
import { PipelineTotals, type MetricPoint, type RouteOverviewMetrics } from '../../../domain';
import type { FlowCounts } from './RouteDetailModel';
import type { InOutPoint, LatencyPoint, PartitionLag } from './MetricSeries';

/** Counts of one branch strip. */
export type BranchCounts = { branch: string; counts: FlowCounts };

/** Everything the four overview charts draw. */
export type OverviewChartData = { io: InOutPoint[]; lag: PartitionLag[]; latency: LatencyPoint[]; errors: number[] };

/** Turns server overview metrics into pipeline counts and chart-ready values (bucketed for readability). */
export class OverviewSeriesView {
  static readonly buckets = 36;
  static readonly percent = 100;
  static readonly msPerSecond = 1_000;
  static readonly minBarPct = 3;
  static readonly partitionPrefix = 'P';
  static readonly decimals = 1;

  static counts(metrics: RouteOverviewMetrics): FlowCounts {
    const t = PipelineTotals.of(metrics.pipeline);
    return { read: Math.round(t.read), passed: Math.round(t.passed), sent: Math.round(t.sent), deadLetters: Math.round(t.dlq) };
  }

  /** One strip per branch when the route has several (P2); null for single-branch routes (one strip with totals). */
  static branchCounts(metrics: RouteOverviewMetrics): BranchCounts[] | null {
    if (metrics.pipeline.length < 2) return null;
    return metrics.pipeline.map((row) => ({ branch: row.branch, counts: OverviewSeriesView.counts({ ...metrics, pipeline: [row] }) }));
  }

  /** True when the range holds no samples at all (route idle, or not scraped yet). */
  static isEmpty(metrics: RouteOverviewMetrics): boolean {
    const s = metrics.series;
    const lists = [s.eventsIn, s.eventsOut, s.latencyP50, s.latencyP95, s.errorsPerMin, ...Object.values(s.lagByPartition)];
    return metrics.pipeline.length === 0 && lists.every((points) => points.length === 0);
  }

  static chartData(metrics: RouteOverviewMetrics): OverviewChartData {
    const s = metrics.series;
    const ins = OverviewSeriesView.bucket(s.eventsIn);
    const outs = OverviewSeriesView.bucket(s.eventsOut);
    const peak = Math.max(1, ...ins, ...outs);
    const pct = (v: number) => Math.round((v / peak) * OverviewSeriesView.percent);
    const p50 = OverviewSeriesView.bucket(s.latencyP50);
    const p95 = OverviewSeriesView.bucket(s.latencyP95);
    const ms = (v: number) => Math.round(v * OverviewSeriesView.msPerSecond);
    return {
      io: ins.map((v, i) => ({ inPct: pct(v), outPct: pct(outs[i] ?? 0) })),
      lag: OverviewSeriesView.partitions(s.lagByPartition),
      latency: p50.map((v, i) => ({ p50: ms(v), p95: ms(p95[i] ?? 0) })),
      errors: OverviewSeriesView.bucket(s.errorsPerMin).map((v) => Number(v.toFixed(OverviewSeriesView.decimals))),
    };
  }

  /** Averages points into at most `buckets` values. */
  static bucket(points: MetricPoint[]): number[] {
    if (points.length <= OverviewSeriesView.buckets) return points.map(([, v]) => v);
    const size = points.length / OverviewSeriesView.buckets;
    return Array.from({ length: OverviewSeriesView.buckets }, (_, i) => {
      const slice = points.slice(Math.floor(i * size), Math.floor((i + 1) * size));
      return slice.reduce((sum, [, v]) => sum + v, 0) / Math.max(1, slice.length);
    });
  }

  private static partitions(byPartition: Record<string, MetricPoint[]>): PartitionLag[] {
    const latest = Object.entries(byPartition).map(([partition, points]) => ({ partition, lag: Math.round(points.at(-1)?.[1] ?? 0) }));
    const max = Math.max(1, ...latest.map((p) => p.lag));
    return latest.map(({ partition, lag }) => ({
      partition: /^\d+$/.test(partition) ? `${OverviewSeriesView.partitionPrefix}${partition}` : partition,
      lag,
      heightPct: Math.max(OverviewSeriesView.minBarPct, (lag / max) * OverviewSeriesView.percent),
      label: Format.number(lag),
    }));
  }
}
