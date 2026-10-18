import { MetricsRange, type MetricPoint, type RouteOverviewMetrics } from '../../domain';
import { MockTime } from '../../mocks';

/** Window length and step (seconds) of each overview range, as the server uses them. */
const RangeShape: Record<MetricsRange, { seconds: number; step: number; label: string }> = {
  [MetricsRange.M15]: { seconds: 900, step: 15, label: '15s' },
  [MetricsRange.H1]: { seconds: 3_600, step: 30, label: '30s' },
  [MetricsRange.H24]: { seconds: 86_400, step: 300, label: '5m' },
  [MetricsRange.D7]: { seconds: 604_800, step: 3_600, label: '1h' },
};

/** Deterministic overview series seeded by route id (mirrors the API's mock metrics backend). */
export class MockRouteMetrics {
  static readonly partitions = 3;
  static readonly branch = 'main';
  static readonly maxBase = 1_000;
  static readonly swing = 0.3;
  static readonly periodSec = 3_600;
  static readonly msPerSec = 1_000;
  static readonly matchShare = 0.8;
  static readonly dlqShare = 0.01;
  static readonly latencyScale = 0.0001;

  static build(routeId: string, range: MetricsRange): RouteOverviewMetrics {
    const shape = RangeShape[range];
    const end = Math.floor(MockTime.reference / MockRouteMetrics.msPerSec);
    const series = (key: string, scale = 1) => MockRouteMetrics.series(`${routeId}:${key}`, end, shape.seconds, shape.step, scale);
    const lagByPartition: Record<string, MetricPoint[]> = {};
    for (let p = 0; p < MockRouteMetrics.partitions; p++) lagByPartition[String(p)] = series(`lag${p}`);
    const read = Math.round(MockRouteMetrics.base(`${routeId}:in`) * shape.seconds);
    const passed = Math.round(read * MockRouteMetrics.matchShare);
    const dlq = Math.round(passed * MockRouteMetrics.dlqShare);
    return {
      range,
      step: shape.label,
      pipeline: [{ branch: MockRouteMetrics.branch, read, passed, sent: passed - dlq, dlq }],
      series: {
        eventsIn: series('in'),
        eventsOut: series('in', MockRouteMetrics.matchShare),
        lagByPartition,
        latencyP50: series('p50', MockRouteMetrics.latencyScale),
        latencyP95: series('p95', MockRouteMetrics.latencyScale * 3),
        errorsPerMin: series('err', MockRouteMetrics.dlqShare),
      },
    };
  }

  private static series(seed: string, end: number, seconds: number, step: number, scale: number): MetricPoint[] {
    const base = MockRouteMetrics.base(seed) * scale;
    const phase = MockRouteMetrics.hash(seed) % 360;
    const points: MetricPoint[] = [];
    for (let t = end - seconds; t <= end; t += step) {
      const wave = Math.sin((2 * Math.PI * t) / MockRouteMetrics.periodSec + (phase * Math.PI) / 180);
      points.push([t, Math.round(base * (1 + MockRouteMetrics.swing * wave) * 1_000) / 1_000]);
    }
    return points;
  }

  private static base(seed: string): number {
    return (MockRouteMetrics.hash(seed) % MockRouteMetrics.maxBase) + 1;
  }

  private static hash(seed: string): number {
    let h = 2_166_136_261;
    for (let i = 0; i < seed.length; i++) h = Math.imul(h ^ seed.charCodeAt(i), 16_777_619);
    return h >>> 0;
  }
}
