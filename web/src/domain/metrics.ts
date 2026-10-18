/** Overview time ranges served by `GET /routes/{id}/metrics?range=` (spec §10.1). */
export const MetricsRange = { M15: '15m', H1: '1h', H24: '24h', D7: '7d' } as const;
export type MetricsRange = (typeof MetricsRange)[keyof typeof MetricsRange];

/** One chart point: `[unixSeconds, value]`. */
export type MetricPoint = [number, number];

/** Counts along one branch of the pipeline strip: read → passed filter → sent, plus dead letters. */
export type PipelineCounts = { branch: string; read: number; passed: number; sent: number; dlq: number };

/** The four overview charts of one route; latencies are in seconds. */
export type RouteMetricSeries = {
  eventsIn: MetricPoint[];
  eventsOut: MetricPoint[];
  lagByPartition: Record<string, MetricPoint[]>;
  latencyP50: MetricPoint[];
  latencyP95: MetricPoint[];
  errorsPerMin: MetricPoint[];
};

/** Overview metrics of one route for a range. */
export type RouteOverviewMetrics = { range: MetricsRange; step: string; pipeline: PipelineCounts[]; series: RouteMetricSeries };

/** Server-configured consumer-lag thresholds (returned with the dashboard summary). */
export type LagThresholds = { routeLagWarn: number; routeLagError: number; totalLagWarn: number; totalLagError: number };

/** Sums branch rows into one strip (MVP shows a single row). */
export class PipelineTotals {
  static of(rows: PipelineCounts[]): Omit<PipelineCounts, 'branch'> {
    return rows.reduce(
      (sum, r) => ({ read: sum.read + r.read, passed: sum.passed + r.passed, sent: sum.sent + r.sent, dlq: sum.dlq + r.dlq }),
      { read: 0, passed: 0, sent: 0, dlq: 0 },
    );
  }
}
