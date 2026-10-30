import {
  ActivityVerb,
  BulkAction,
  DeliveryMode,
  InFlightMode,
  DeploymentStatus,
  DiffMark,
  DiffSection,
  IssueSeverity,
  DraftSummary,
  RouteDraftFactory,
  RouteSettingsRules,
  RouteStatus,
  TemplateCatalog,
  VersionDiff,
  type Activity,
  type BulkItemResult,
  type Deployment,
  type DocumentFormat,
  type ImportOptions,
  type ImportReport,
  type Replay,
  type ReplayRequest,
  type RouteExport,
  type Issue,
  type Route,
  type RouteDiff,
  type RouteDraft,
  type RouteSettings,
  type RouteSettingsPatch,
  type RouteTemplateId,
  type RouteVersion,
  type ValidationMode,
  type ValidationResult,
} from '../../domain';
import { CURRENT_USER, DraftPresets, MOCK_ROUTE_DEFAULTS, VERSION_TEXT } from '../../mocks';
import { RoutesService, type DeployResult } from '../contracts';
import type { MockBackend, MockDatabase } from './MockBackend';
import { MockRoutePortability } from './MockRoutePortability';
import { MockRouteReplays } from './MockRouteReplays';

/** In-memory routes backend reproducing the design's pause/duplicate/deploy/rollback behaviour. */
export class MockRoutesService extends RoutesService {
  private readonly base: MockRouteStore;
  private readonly savedSettings = new Map<string, RouteSettings>();
  private readonly replays: MockRouteReplays;

  constructor(backend: MockBackend) {
    super();
    this.base = new MockRouteStore(backend);
    this.replays = new MockRouteReplays(backend);
  }

  list(): Promise<Route[]> {
    return this.base.reply(this.base.live());
  }

  get(id: string): Promise<Route | undefined> {
    return this.base.reply(this.base.find(id));
  }

  pause(id: string): Promise<Route> {
    return this.base.reply(this.base.patch(id, { status: RouteStatus.Paused }));
  }

  resume(id: string): Promise<Route> {
    const route = this.base.require(id);
    const eventsPerSec = route.metrics.eventsPerSec || MOCK_ROUTE_DEFAULTS.resumedEventsPerSec;
    return this.base.reply(this.base.patch(id, { status: RouteStatus.Running, metrics: { ...route.metrics, eventsPerSec } }));
  }

  duplicate(id: string): Promise<Route> {
    return this.base.reply(this.base.duplicate(id));
  }

  async archive(id: string): Promise<void> {
    this.base.patch(id, { status: RouteStatus.Archived });
    await this.base.reply(null);
  }

  openDraft(routeId: string | null, templateId?: RouteTemplateId | null): Promise<RouteDraft> {
    const route = routeId ? (this.base.find(routeId) ?? null) : null;
    const preset = route ? DraftPresets.forRoute(route.id) : null;
    return this.base.reply(RouteDraftFactory.create(route, preset, TemplateCatalog.find(templateId)));
  }

  saveDraft(draft: RouteDraft): Promise<RouteDraft> {
    return this.base.reply(draft);
  }

  deploy(draft: RouteDraft, note: string): Promise<DeployResult> {
    const route = draft.routeId ? this.base.redeploy(draft) : this.base.createFromDraft(draft);
    const version = this.base.recordVersion(route, note, draft.changes);
    const activity = this.base.recordActivity(route, `v${route.version}`);
    return this.base.reply({ route, version, activity });
  }

  listVersions(routeId: string): Promise<RouteVersion[]> {
    return this.base.reply(this.base.history(routeId));
  }

  rollback(routeId: string, version: number): Promise<RouteVersion> {
    return this.base.reply(this.base.rollback(routeId, version));
  }

  validate(_routeId: string, draft: RouteDraft | null, _mode: ValidationMode): Promise<ValidationResult> {
    const issues = draft ? MockValidator.issues(draft) : [];
    return this.base.reply({ valid: !issues.some((i) => i.severity === IssueSeverity.Error), issues });
  }

  diff(routeId: string, a: string, b: string): Promise<RouteDiff> {
    const target = this.base.history(routeId).find((v) => String(v.version) === b);
    const lines = (target?.diff ?? []).map((l) => ({ ...l, path: '' }));
    const sections = Object.values(DiffSection).map((section) => ({ section, lines: section === DiffSection.Settings ? lines : [] }));
    return this.base.reply({ a, b, sections });
  }

  listDeployments(routeId: string): Promise<Deployment[]> {
    const deployments = this.base.history(routeId).map((v, i) => ({
      id: `d-${routeId}-${v.version}`,
      versionNo: v.version,
      status: i === 0 ? DeploymentStatus.Active : DeploymentStatus.Superseded,
      failureReason: '',
      requestedAt: v.createdAt,
      finishedAt: v.createdAt,
    }));
    return this.base.reply(deployments);
  }

  settings(routeId: string): Promise<RouteSettings> {
    return this.base.reply(this.settingsOf(routeId));
  }

  updateSettings(routeId: string, patch: RouteSettingsPatch): Promise<RouteSettings> {
    const next = { ...this.settingsOf(routeId), ...patch };
    this.savedSettings.set(routeId, next);
    this.base.patch(routeId, { name: next.name, tags: next.tags });
    return this.base.reply(next);
  }

  private settingsOf(routeId: string): RouteSettings {
    const route = this.base.require(routeId);
    const defaults: RouteSettings = {
      name: route.name,
      description: '',
      tags: route.tags ?? [],
      replicas: MOCK_ROUTE_DEFAULTS.replicas,
      errorPolicy: RouteSettingsRules.defaultErrorPolicy,
      deliveryMode: DeliveryMode.AtLeastOnce,
      inFlightMode: InFlightMode.Auto,
      limits: RouteSettingsRules.emptyLimits,
    };
    return this.savedSettings.get(routeId) ?? defaults;
  }

  listArchived(): Promise<Route[]> {
    return this.base.reply(this.base.archived());
  }

  unarchive(id: string): Promise<Route> {
    return this.base.reply(this.base.patch(id, { status: RouteStatus.Paused }));
  }

  exportRoutes(ids: string[], format: DocumentFormat): Promise<RouteExport> {
    return this.base.reply(MockRoutePortability.export(ids.map((id) => this.base.require(id)), format));
  }

  importRoutes(content: string, options: ImportOptions): Promise<ImportReport> {
    const create = (doc: { name: string }) => this.base.patch(this.base.createFromDraft({ ...RouteDraftFactory.create(null, null), name: doc.name }).id, { status: MockRoutePortability.newRouteStatus });
    return this.base.reply(MockRoutePortability.import(content, options, this.base.all(), create));
  }

  replay(routeId: string, request: ReplayRequest): Promise<Replay> {
    this.base.require(routeId);
    return this.replays.request(routeId, request);
  }

  replayStatus(routeId: string, replayId: string): Promise<Replay> {
    return this.replays.status(routeId, replayId);
  }

  bulk(action: BulkAction, ids: string[]): Promise<BulkItemResult[]> {
    const status = action === BulkAction.Pause ? RouteStatus.Paused : RouteStatus.Running;
    const results = ids.map((id) => {
      if (!this.base.find(id)) return { id, ok: false, message: 'This route does not exist.' };
      if (action !== BulkAction.AddTag) this.base.patch(id, { status });
      return { id, ok: true, message: null };
    });
    return this.base.reply(results);
  }
}

/** Client-side stand-in for the server's draft validation (mock data source only). */
class MockValidator {
  static readonly messages = {
    noSource: 'Pick a source connection.',
    noTopic: 'Pick or type a destination topic.',
  } as const;

  static issues(draft: RouteDraft): Issue[] {
    const issues: Issue[] = [];
    if (!draft.source.connectionId) issues.push(MockValidator.error('/source/connectionId', MockValidator.messages.noSource));
    if (!draft.destination.topic) issues.push(MockValidator.error('/branches/0/destination/topic', MockValidator.messages.noTopic));
    return issues;
  }

  private static error(path: string, message: string): Issue {
    return { code: 'schema_invalid', severity: IssueSeverity.Error, path, message };
  }
}

/** Mutation steps on the mock database used by MockRoutesService. */
class MockRouteStore {
  private readonly backend: MockBackend;

  constructor(backend: MockBackend) {
    this.backend = backend;
  }

  private get db(): MockDatabase {
    return this.backend.db;
  }

  reply<T>(value: T): Promise<T> {
    return this.backend.reply(value);
  }

  live(): Route[] {
    return this.db.routes.filter((r) => r.status !== RouteStatus.Archived);
  }

  archived(): Route[] {
    return this.db.routes.filter((r) => r.status === RouteStatus.Archived);
  }

  all(): Route[] {
    return this.db.routes;
  }

  find(id: string): Route | undefined {
    return this.db.routes.find((r) => r.id === id);
  }

  require(id: string): Route {
    const route = this.find(id);
    if (!route) throw new Error(`Route ${id} not found`);
    return route;
  }

  patch(id: string, patch: Partial<Route>): Route {
    const next = { ...this.require(id), ...patch };
    this.db.routes = this.db.routes.map((r) => (r.id === id ? next : r));
    return next;
  }

  duplicate(id: string): Route {
    const source = this.require(id);
    const copy: Route = {
      ...structuredClone(source),
      id: `${source.id}c${Date.now() % MOCK_ROUTE_DEFAULTS.copyIdModulo}`,
      name: `${source.name} (copy)`,
      status: RouteStatus.Draft,
      metrics: { ...source.metrics, eventsPerSec: 0 },
      updatedAt: this.backend.nowIso(),
    };
    this.db.routes = [copy, ...this.db.routes];
    return copy;
  }

  redeploy(draft: RouteDraft): Route {
    const current = this.require(draft.routeId as string);
    return this.patch(current.id, {
      ...this.fromDraft(draft),
      version: current.version + 1,
      metrics: { ...current.metrics, eventsPerSec: current.metrics.eventsPerSec || MOCK_ROUTE_DEFAULTS.deployedEventsPerSec },
    });
  }

  createFromDraft(draft: RouteDraft): Route {
    const route: Route = {
      id: `r${Date.now() % MOCK_ROUTE_DEFAULTS.idModulo}`,
      owner: MOCK_ROUTE_DEFAULTS.newRouteOwner,
      ...this.fromDraft(draft),
      destination: { connectionId: draft.destination.connectionId, topic: draft.destination.topic || MOCK_ROUTE_DEFAULTS.newRouteTopic },
      metrics: { eventsPerSec: MOCK_ROUTE_DEFAULTS.deployedEventsPerSec, matchPct: MOCK_ROUTE_DEFAULTS.newRouteMatchPct, lag: 0 },
      version: 1,
    };
    this.db.routes = [route, ...this.db.routes];
    return route;
  }

  private fromDraft(draft: RouteDraft): Omit<Route, 'id' | 'owner' | 'metrics' | 'version'> {
    return {
      name: draft.name,
      status: RouteStatus.Running,
      source: { connectionId: draft.source.connectionId, topics: draft.source.topics.slice() },
      destination: { connectionId: draft.destination.connectionId, topic: draft.destination.topic },
      filterSummary: DraftSummary.filterSummary(draft.filter),
      transformMode: DraftSummary.transformMode(draft.steps),
      updatedAt: this.backend.nowIso(),
    };
  }

  history(routeId: string): RouteVersion[] {
    return this.db.historyOf(routeId);
  }

  recordVersion(route: Route, note: string, changes: string[]): RouteVersion {
    const texts = changes.length ? changes : [VERSION_TEXT.unchanged];
    const version: RouteVersion = {
      routeId: route.id,
      version: route.version,
      author: CURRENT_USER.name,
      createdAt: this.backend.nowIso(),
      note: note || VERSION_TEXT.noNote,
      diff: texts.map((text) => ({ mark: DiffMark.Changed, text })),
    };
    this.history(route.id).unshift(version);
    return version;
  }

  recordActivity(route: Route, detail: string): Activity {
    const activity: Activity = {
      id: `a${Date.now()}`,
      actor: CURRENT_USER.name,
      verb: ActivityVerb.Deployed,
      routeName: route.name,
      detail,
      at: this.backend.nowIso(),
    };
    this.db.activity.unshift(activity);
    return activity;
  }

  rollback(routeId: string, versionNo: number): RouteVersion {
    const history = this.history(routeId);
    const target = history.find((v) => v.version === versionNo);
    if (!target) throw new Error(`Version ${versionNo} not found`);
    const version: RouteVersion = {
      routeId,
      version: history[0].version + 1,
      author: CURRENT_USER.name,
      createdAt: this.backend.nowIso(),
      note: `Rolled back to v${versionNo}`,
      diff: VersionDiff.invert(target.diff),
    };
    history.unshift(version);
    return version;
  }
}
