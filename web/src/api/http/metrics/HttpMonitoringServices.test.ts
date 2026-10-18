import { afterEach, describe, expect, it } from 'vitest';
import { ActivityVerb, IntentType, LagHealth, LagThreshold, MetricsRange, Tone } from '../../../domain';
import { HttpActivityService } from '../activity/HttpActivityService';
import { HttpAlertsService } from '../alerts/HttpAlertsService';
import { HttpContext } from '../core/HttpContext';
import { TestFetch } from '../core/TestFetch.test-util';
import { HttpMetricsService } from './HttpMetricsService';

const context = (fetch: TestFetch) => new HttpContext({ apiBaseUrl: '/api/v1', authBaseUrl: '', fetch: fetch.fn });
const designThresholds = {
  routeLagWarn: LagThreshold.RouteWarn,
  routeLagError: LagThreshold.RouteError,
  totalLagWarn: LagThreshold.TotalWarn,
  totalLagError: LagThreshold.TotalError,
};

const summary = {
  routes: { running: 3, paused: 1, error: 1, draft: 2 },
  eventsLastHour: { total: 1234.6, spark: [1, 2, 3] },
  totalLag: { value: 900, tone: Tone.Ok },
  dlq24h: { value: 4 },
  thresholds: { routeLagWarn: 100, routeLagError: 500, totalLagWarn: 1_000, totalLagError: 5_000 },
  metricsAvailable: true,
};

afterEach(() => LagHealth.configure(designThresholds));

describe('HttpMetricsService', () => {
  it('maps the summary and adopts the server lag thresholds', async () => {
    const fetch = new TestFetch().json(200, summary);
    const stats = await new HttpMetricsService(context(fetch)).dashboard();
    expect(fetch.calls[0].url).toBe('/api/v1/dashboard/summary');
    expect(stats).toMatchObject({ eventsLastHour: 1235, deadLetters24h: 4, metricsAvailable: true, spark: [1, 2, 3] });
    expect(LagHealth.routeTone(200)).toBe(Tone.Warn);
    expect(LagHealth.routeTone(600)).toBe(Tone.Err);
    expect(LagHealth.totalTone(6_000)).toBe(Tone.Err);
    expect(LagHealth.totalNote(6_000)).toBe('Above 5k threshold');
  });

  it('keeps metric cards empty when the metrics store is down', async () => {
    const fetch = new TestFetch().json(200, { ...summary, eventsLastHour: null, totalLag: null, dlq24h: null, metricsAvailable: false });
    const stats = await new HttpMetricsService(context(fetch)).dashboard();
    expect(stats).toMatchObject({ eventsLastHour: 0, totalLag: null, deadLetters24h: null, metricsAvailable: false });
  });

  it('requests route metrics with the range and surfaces 503', async () => {
    const body = { range: '24h', step: '5m', pipeline: [], series: { eventsIn: [], eventsOut: [], lagByPartition: {}, latencyP50: [], latencyP95: [], errorsPerMin: [] } };
    const fetch = new TestFetch().json(200, body).problem(503, { type: 'x', title: 'Unavailable', status: 503, code: 'metrics_unavailable' });
    const svc = new HttpMetricsService(context(fetch));
    expect((await svc.routeMetrics('r/1', MetricsRange.H24)).step).toBe('5m');
    expect(fetch.calls[0].url).toBe('/api/v1/routes/r%2F1/metrics?range=24h');
    const error = await TestFetch.failure(svc.routeMetrics('r1', MetricsRange.H1));
    expect(error.status).toBe(503);
  });
});

describe('HttpAlertsService', () => {
  it('maps attention rows to the card shape with their intents', async () => {
    const intent = { type: IntentType.TestConnection, connectionId: 'c1' };
    const fetch = new TestFetch().json(200, {
      items: [{ id: 'connection_unhealthy:c1', kind: 'connection_unhealthy', tone: Tone.Err, title: 'prod is unreachable', detail: 'd', action: { label: 'Test', intent } }],
    });
    const items = await new HttpAlertsService(context(fetch)).attention();
    expect(items).toEqual([{ id: 'connection_unhealthy:c1', kind: 'connection_unhealthy', tone: Tone.Err, title: 'prod is unreachable', detail: 'd', actionLabel: 'Test', intent }]);
    const empty = new TestFetch().json(200, { items: [], next_cursor: null, unread: 0 });
    expect(await new HttpAlertsService(context(empty)).list()).toEqual([]);
    expect(empty.calls[0].url).toBe('/api/v1/alerts?limit=20');
  });
});

describe('HttpActivityService', () => {
  it('reads the last 10 route changes and maps verbs', async () => {
    const fetch = new TestFetch().json(200, {
      items: [
        { id: '9', actor: 'Ana', verb: 'rolled_back', routeId: 'r1', routeName: 'orders', detail: '', at: '2026-10-13T09:00:00Z' },
        { id: '8', actor: 'Bo', verb: 'resumed', routeId: 'r2', routeName: 'pay', detail: '', at: '2026-10-13T08:00:00Z' },
      ],
      next_cursor: null,
    });
    const list = await new HttpActivityService(context(fetch)).list();
    expect(fetch.calls[0].url).toBe('/api/v1/activity?limit=10');
    expect(list.map((a) => a.verb)).toEqual([ActivityVerb.RolledBack, ActivityVerb.Resumed]);
  });
});
