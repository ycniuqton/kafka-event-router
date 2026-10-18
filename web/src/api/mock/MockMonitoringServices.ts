import {
  AlertState,
  AlertStateTone,
  ChannelTypeMeta,
  FlowNodeType,
  IntentType,
  PushEventType,
  PushState,
  RouteDetailTab,
  RouteStatus,
  type Alert,
  type AlertFeed,
  type AlertRule,
  type AlertRuleSave,
  type AttentionItem,
  type ChannelTestResult,
  type DashboardStats,
  type FlowEdge,
  type FlowMap,
  type FlowMapQuery,
  type FlowNode,
  type MetricsRange,
  type PushEvent,
  type Route,
  type RouteOverviewMetrics,
} from '../../domain';
import { ALERTS, ATTENTION, DASHBOARD_STATS } from '../../mocks';
import { AlertsService, MetricsService, PushEventsService } from '../contracts';
import type { MockBackend } from './MockBackend';
import { MockRouteMetrics } from './MockRouteMetrics';

/** Timing of the mock push stream: one alert fires shortly after the first subscriber connects. */
export const MockPushTiming = { firstFireMs: 3000 } as const;

/** In-memory alert rules and bell feed shared by the mock alerts and push services. */
export class MockAlertBook {
  readonly rules = new Map<string, AlertRule>();
  feed: Alert[] = structuredClone(ALERTS).map((a) => ({ ...a, state: AlertState.Firing, acked: false }));
  private readonly acked = new Set<string>();
  private seq = 0;

  nextId(prefix: string): string {
    this.seq += 1;
    return `${prefix}${this.seq}`;
  }

  unread(): number {
    return this.feed.filter((a) => !a.acked && !this.acked.has(a.id)).length;
  }

  ack(id: string): void {
    this.acked.add(id);
    this.feed = this.feed.map((a) => (a.id === id ? { ...a, acked: true } : a));
  }

  /** Adds a firing alert for route (used by the push simulation). */
  fire(route: Route, at: string): Alert {
    const alert: Alert = {
      id: this.nextId('mal'),
      tone: AlertStateTone[AlertState.Firing],
      title: `${route.name} · No events sent`,
      detail: 'No events sent for 10 min',
      intent: { type: IntentType.RouteDetail, routeId: route.id, tab: RouteDetailTab.Overview },
      state: AlertState.Firing,
      acked: false,
      at,
    };
    this.feed = [alert, ...this.feed];
    return alert;
  }
}

/** Alerts, rules and the bell feed over the in-memory book. */
export class MockAlertsService extends AlertsService {
  private readonly backend: MockBackend;
  private readonly book: MockAlertBook;

  constructor(backend: MockBackend, book: MockAlertBook) {
    super();
    this.backend = backend;
    this.book = book;
  }

  list(): Promise<Alert[]> {
    return this.backend.reply(this.book.feed);
  }

  attention(): Promise<AttentionItem[]> {
    return this.backend.reply(ATTENTION);
  }

  feed(): Promise<AlertFeed> {
    return this.backend.reply({ items: this.book.feed, unread: this.book.unread() });
  }

  ack(id: string): Promise<void> {
    this.book.ack(id);
    return this.backend.reply(undefined);
  }

  rules(routeId: string): Promise<AlertRule[]> {
    return this.backend.reply([...this.book.rules.values()].filter((r) => r.routeId === routeId));
  }

  saveRule({ draft, existing }: AlertRuleSave): Promise<AlertRule> {
    const previous = existing ? this.book.rules.get(existing.id) : undefined;
    if (existing && previous?.version !== existing.version) return this.backend.fail('Someone else changed this. Reload to see their version, then retry.');
    const channels = draft.channels.map((c, i) => ({
      type: c.type,
      target: c.target,
      secretSet: ChannelTypeMeta[c.type].usesSecret && (Boolean(c.secret) || Boolean(previous?.channels[i]?.secretSet)),
    }));
    const rule: AlertRule = { ...draft, id: existing?.id ?? this.book.nextId('rule'), channels, version: (previous?.version ?? 0) + 1 };
    this.book.rules.set(rule.id, rule);
    return this.backend.reply(rule);
  }

  deleteRule(id: string): Promise<void> {
    this.book.rules.delete(id);
    return this.backend.reply(undefined);
  }

  testRule(id: string): Promise<ChannelTestResult[]> {
    const rule = this.book.rules.get(id);
    return this.backend.reply((rule?.channels ?? []).map((c) => ({ channel: c.type, ok: true })));
  }
}

/** Dashboard stats, route overview metrics and the flow map from the mock routes. */
export class MockMetricsService extends MetricsService {
  private readonly backend: MockBackend;

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  dashboard(): Promise<DashboardStats> {
    return this.backend.reply(DASHBOARD_STATS);
  }

  routeMetrics(routeId: string, range: MetricsRange): Promise<RouteOverviewMetrics> {
    return this.backend.reply(MockRouteMetrics.build(routeId, range));
  }

  flowMap(query: FlowMapQuery): Promise<FlowMap> {
    return this.backend.reply(MockFlowMap.build(this.backend.db.routes, query));
  }
}

/** Builds the server flow-map shape from mock routes (one branch per route). */
export class MockFlowMap {
  static readonly branch = 'main';

  static build(routes: Route[], query: FlowMapQuery): FlowMap {
    const nodes = new Map<string, FlowNode>();
    const edges: FlowEdge[] = [];
    routes.filter((r) => r.status !== RouteStatus.Draft && MockFlowMap.matches(r, query)).forEach((r) => {
      const to = MockFlowMap.topic(nodes, r.destination.connectionId, r.destination.topic);
      r.source.topics.forEach((t) => {
        const from = MockFlowMap.topic(nodes, r.source.connectionId, t);
        edges.push({ id: `${r.id}:${t}`, routeId: r.id, routeName: r.name, teamName: r.owner, branch: MockFlowMap.branch, from, to,
          eventsPerSec: r.metrics.eventsPerSec, status: r.status });
      });
    });
    return { nodes: [...nodes.values()], edges, metricsAvailable: true };
  }

  private static matches(r: Route, q: FlowMapQuery): boolean {
    const ends = [r.source.connectionId, r.destination.connectionId];
    return (!q.connection || ends.includes(q.connection)) && (!q.team || q.team === r.owner) && (!q.tag || (r.tags ?? []).includes(q.tag) || ends.some((e) => e.startsWith(q.tag)));
  }

  private static topic(nodes: Map<string, FlowNode>, connectionId: string, topic: string): string {
    const id = `topic:${connectionId}:${topic}`;
    nodes.set(id, { id, type: FlowNodeType.Topic, label: topic, connectionId });
    return id;
  }
}

/** Timer-driven push stream: `hello`, then one `alert.fired` for the first running route. */
export class MockPushEventsService extends PushEventsService {
  private readonly backend: MockBackend;
  private readonly book: MockAlertBook;
  private readonly listeners = new Set<(e: PushEvent) => void>();
  private readonly stateListeners = new Set<(s: PushState) => void>();
  private timer: ReturnType<typeof setTimeout> | undefined;
  private current: PushState = PushState.Connecting;

  constructor(backend: MockBackend, book: MockAlertBook) {
    super();
    this.backend = backend;
    this.book = book;
  }

  subscribe(listener: (event: PushEvent) => void): () => void {
    this.listeners.add(listener);
    if (this.listeners.size === 1) this.start();
    return () => {
      this.listeners.delete(listener);
      if (this.listeners.size === 0) clearTimeout(this.timer);
    };
  }

  onState(listener: (state: PushState) => void): () => void {
    this.stateListeners.add(listener);
    return () => this.stateListeners.delete(listener);
  }

  state(): PushState {
    return this.current;
  }

  private start(): void {
    this.setState(PushState.Live);
    this.emit({ type: PushEventType.Hello, at: this.backend.nowIso(), payload: {} });
    this.timer = setTimeout(() => this.fireOne(), MockPushTiming.firstFireMs);
  }

  private fireOne(): void {
    const route = this.backend.db.routes.find((r) => r.status === RouteStatus.Running);
    if (!route) return;
    const alert = this.book.fire(route, this.backend.nowIso());
    this.emit({ type: PushEventType.AlertFired, at: alert.at ?? this.backend.nowIso(), payload: { routeId: route.id, routeName: route.name, ruleName: alert.title } });
  }

  private emit(event: PushEvent): void {
    this.listeners.forEach((l) => l(event));
  }

  private setState(state: PushState): void {
    this.current = state;
    this.stateListeners.forEach((l) => l(state));
  }
}

/** The monitoring mock services sharing one alert book. */
export class MockMonitoring {
  readonly alerts: MockAlertsService;
  readonly metrics: MockMetricsService;
  readonly push: MockPushEventsService;

  constructor(backend: MockBackend) {
    const book = new MockAlertBook();
    this.alerts = new MockAlertsService(backend, book);
    this.metrics = new MockMetricsService(backend);
    this.push = new MockPushEventsService(backend, book);
  }
}
