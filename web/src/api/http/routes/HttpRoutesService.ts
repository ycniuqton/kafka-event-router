import {
  RouteDraftFactory,
  RouteStatus,
  TemplateCatalog,
  type BulkAction,
  type DocumentFormat,
  type ImportOptions,
  type ImportReport,
  type Replay,
  type ReplayRequest,
  type RouteExport,
  type BulkItemResult,
  type Deployment,
  type Route,
  type RouteDiff,
  type RouteDraft,
  type RouteSettings,
  type RouteSettingsPatch,
  type RouteTemplateId,
  type RouteVersion,
  type ValidationMode,
  type ValidationResult,
} from '../../../domain';
import { RouteDraftMapper, type DraftSnapshot } from '../../../domain/routeConfig/RouteDraftMapper';
import { RoutesService, type DeployResult } from '../../contracts';
import { ApiError } from '../core/ApiError';
import { CursorIterator, PageLimit } from '../core/CursorIterator';
import { HttpMethod, HttpStatus } from '../core/HttpConstants';
import type { HttpContext } from '../core/HttpContext';
import {
  RouteAction,
  RouteEndpoint,
  RouteQuery,
  type ImportReportDto,
  type ReplayDto,
  type BulkResultDto,
  type DeployResultDto,
  type DeploymentDto,
  type RouteDetailDto,
  type RouteDiffDto,
  type RouteSummaryDto,
  type VersionSummaryDto,
} from './RouteDtos';
import { NewDraftConnections } from './NewDraftConnections';
import { RouteMapper } from './RouteMapper';
import { RoutePortabilityMapper } from './RoutePortabilityMapper';
import { RouteSettingsMapper } from './RouteSettingsMapper';

/**
 * `services.routes` over `/api/v1/routes`: list/detail, drafts saved with If-Match on `row_version`,
 * deploy/rollback, versions with readable diffs, deployments and bulk actions.
 */
export class HttpRoutesService extends RoutesService {
  private readonly context: HttpContext;
  private readonly snapshots = new Map<string, DraftSnapshot>();
  private readonly settingsBase = new Map<string, { dto: RouteDetailDto; etag: string | null }>();

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async list(): Promise<Route[]> {
    const pages = new CursorIterator<RouteSummaryDto>(this.context.api, RouteEndpoint.collection, { limit: PageLimit.Max });
    return (await pages.all()).map((dto) => RouteMapper.route(dto));
  }

  async get(id: string): Promise<Route | undefined> {
    try {
      return RouteMapper.route(await this.detail(id));
    } catch (error) {
      if (error instanceof ApiError && error.status === HttpStatus.NotFound) return undefined;
      throw error;
    }
  }

  async pause(id: string): Promise<Route> {
    return this.command(id, RouteAction.Pause);
  }

  async resume(id: string): Promise<Route> {
    return this.command(id, RouteAction.Resume);
  }

  async duplicate(id: string): Promise<Route> {
    return this.command(id, RouteAction.Duplicate);
  }

  async archive(id: string): Promise<void> {
    await this.context.api.delete(RouteEndpoint.item(id));
    this.context.etags.forget(RouteEndpoint.item(id));
    this.snapshots.delete(id);
  }

  async openDraft(routeId: string | null, templateId?: RouteTemplateId | null): Promise<RouteDraft> {
    if (!routeId) return new NewDraftConnections(this.context).apply(RouteDraftFactory.create(null, null, TemplateCatalog.find(templateId)));
    const dto = await this.detail(routeId);
    const doc = dto.draft ?? dto.active;
    if (!doc)
      return {
        ...RouteDraftFactory.create(RouteMapper.route(dto), null),
        routeId,
        status: dto.status,
      };
    const { draft, snapshot } = RouteDraftMapper.fromDocument(doc, {
      routeId,
      name: dto.name,
      status: dto.status,
    });
    this.snapshots.set(routeId, snapshot);
    return draft;
  }

  /** Creates the route on first save, then PATCHes the draft with If-Match (412 when someone else saved). */
  async saveDraft(draft: RouteDraft): Promise<RouteDraft> {
    const snapshot = draft.routeId ? (this.snapshots.get(draft.routeId) ?? null) : null;
    const doc = RouteDraftMapper.toDocument(draft, snapshot);
    const res = draft.routeId
      ? await this.context.api.send<RouteDetailDto>({
          method: HttpMethod.Patch,
          path: RouteEndpoint.item(draft.routeId),
          body: { name: draft.name, draft: doc },
          ifMatch: this.context.etags.ifMatch(RouteEndpoint.item(draft.routeId)),
        })
      : await this.context.api.send<RouteDetailDto>({
          method: HttpMethod.Post,
          path: RouteEndpoint.collection,
          body: { name: draft.name, draft: doc },
        });
    this.remember(res.data, res.etag);
    if (snapshot) this.snapshots.set(res.data.id, { ...snapshot, base: doc });
    return {
      ...draft,
      routeId: res.data.id,
      status: res.data.status,
      changes: [],
    };
  }

  async deploy(draft: RouteDraft, note: string): Promise<DeployResult> {
    const saved = await this.saveDraft(draft);
    const id = saved.routeId as string;
    const res = await this.context.api.send<DeployResultDto>({
      method: HttpMethod.Post,
      path: RouteEndpoint.action(id, RouteAction.Deploy),
      body: { changeNote: note },
      ifMatch: this.context.etags.ifMatch(RouteEndpoint.item(id)),
    });
    return this.deployResult(id, res.data, res.etag);
  }

  async listVersions(routeId: string): Promise<RouteVersion[]> {
    const pages = new CursorIterator<VersionSummaryDto>(this.context.api, RouteEndpoint.action(routeId, RouteAction.Versions), { limit: PageLimit.Max });
    const versions = await pages.all();
    return Promise.all(versions.map(async (v) => RouteMapper.version(routeId, v, await this.versionDiff(routeId, v.no))));
  }

  async rollback(routeId: string, version: number): Promise<RouteVersion> {
    const res = await this.context.api.send<DeployResultDto>({
      method: HttpMethod.Post,
      path: RouteEndpoint.action(routeId, RouteAction.Rollback),
      body: { version },
    });
    return (await this.deployResult(routeId, res.data, res.etag)).version;
  }

  async validate(routeId: string, draft: RouteDraft | null, mode: ValidationMode): Promise<ValidationResult> {
    const body = draft
      ? {
          mode,
          draft: RouteDraftMapper.toDocument(draft, this.snapshots.get(routeId) ?? null),
        }
      : { mode };
    return this.context.api.post<ValidationResult>(RouteEndpoint.action(routeId, RouteAction.Validate), body);
  }

  async diff(routeId: string, a: string, b: string): Promise<RouteDiff> {
    return this.context.api.get<RouteDiffDto>(RouteEndpoint.diff(routeId, a, b));
  }

  async listDeployments(routeId: string): Promise<Deployment[]> {
    const pages = new CursorIterator<DeploymentDto>(this.context.api, RouteEndpoint.action(routeId, RouteAction.Deployments), { limit: PageLimit.Max });
    return (await pages.all()).map((d) => RouteMapper.deployment(d));
  }

  async bulk(action: BulkAction, ids: string[], tag?: string): Promise<BulkItemResult[]> {
    const res = await this.context.api.post<BulkResultDto>(RouteEndpoint.bulkPath, { action, ids, tag });
    return res.results.map((r) => ({
      id: r.id,
      ok: r.ok,
      message: r.problem ? (r.problem.detail ?? r.problem.title) : null,
    }));
  }

  async settings(routeId: string): Promise<RouteSettings> {
    const dto = await this.detail(routeId);
    this.settingsBase.set(routeId, {
      dto,
      etag: this.context.etags.ifMatch(RouteEndpoint.item(routeId)),
    });
    return RouteSettingsMapper.fromDetail(dto);
  }

  /** PATCHes with the ETag of the loaded settings, so a concurrent edit fails with 412 instead of being overwritten. */
  async updateSettings(routeId: string, patch: RouteSettingsPatch): Promise<RouteSettings> {
    if (!this.settingsBase.has(routeId)) await this.settings(routeId);
    const base = this.settingsBase.get(routeId) as {
      dto: RouteDetailDto;
      etag: string | null;
    };
    const res = await this.context.api.send<RouteDetailDto>({
      method: HttpMethod.Patch,
      path: RouteEndpoint.item(routeId),
      body: RouteSettingsMapper.toBody(base.dto, patch),
      ifMatch: base.etag,
    });
    this.remember(res.data, res.etag);
    this.settingsBase.set(routeId, {
      dto: res.data,
      etag: this.context.etags.ifMatch(RouteEndpoint.item(routeId)),
    });
    this.snapshots.delete(routeId);
    return RouteSettingsMapper.fromDetail(res.data);
  }

  async listArchived(): Promise<Route[]> {
    const query = { [RouteQuery.Status]: RouteStatus.Archived };
    const pages = new CursorIterator<RouteSummaryDto>(this.context.api, RouteEndpoint.collection, { limit: PageLimit.Max, query });
    return (await pages.all()).map((dto) => RouteMapper.route(dto));
  }

  async unarchive(id: string): Promise<Route> {
    return this.command(id, RouteAction.Unarchive);
  }

  /** One route: GET its export; several: POST /routes/export (multi-document YAML or a JSON array). */
  async exportRoutes(ids: string[], format: DocumentFormat, version?: string): Promise<RouteExport> {
    const response =
      ids.length === 1
        ? await this.context.api.raw({
            method: HttpMethod.Get,
            path: RouteEndpoint.action(ids[0], RouteAction.Export),
            query: {
              [RouteQuery.Format]: format,
              [RouteQuery.Version]: version,
            },
          })
        : await this.context.api.raw({
            method: HttpMethod.Post,
            path: RouteEndpoint.exportPath,
            body: { ids, format, version },
          });
    return RoutePortabilityMapper.exportFile(response, format);
  }

  /** Sends the file text in the JSON envelope `{content, format}`; the server never deploys imports. */
  async importRoutes(content: string, options: ImportOptions): Promise<ImportReport> {
    const query = {
      [RouteQuery.Mode]: options.mode,
      [RouteQuery.DryRun]: options.dryRun,
    };
    const dto = await this.context.api.post<ImportReportDto>(RouteEndpoint.importPath, { content, format: options.format }, { query });
    return RoutePortabilityMapper.report(dto);
  }

  /** POST /routes/{id}/replay → 202 with the pending replay (422 without `confirm`, 409 while one runs). */
  async replay(routeId: string, request: ReplayRequest): Promise<Replay> {
    const dto = await this.context.api.post<ReplayDto>(RouteEndpoint.action(routeId, RouteAction.Replay), request);
    return RouteMapper.replay(dto);
  }

  async replayStatus(routeId: string, replayId: string): Promise<Replay> {
    return RouteMapper.replay(await this.context.api.get<ReplayDto>(RouteEndpoint.replay(routeId, replayId)));
  }

  private async detail(id: string): Promise<RouteDetailDto> {
    const res = await this.context.api.send<RouteDetailDto>({
      method: HttpMethod.Get,
      path: RouteEndpoint.item(id),
    });
    this.remember(res.data, res.etag);
    return res.data;
  }

  private async command(id: string, action: RouteAction): Promise<Route> {
    const res = await this.context.api.send<RouteDetailDto>({
      method: HttpMethod.Post,
      path: RouteEndpoint.action(id, action),
    });
    this.remember(res.data, res.etag);
    return RouteMapper.route(res.data);
  }

  private remember(dto: RouteDetailDto, etag: string | null): void {
    this.context.etags.remember(RouteEndpoint.item(dto.id), etag ?? `"${dto.revision}"`);
  }

  /** Diff of a version against its predecessor; the first version has none. */
  private async versionDiff(routeId: string, no: number): Promise<RouteDiff | null> {
    if (no <= 1) return null;
    return this.diff(routeId, String(no - 1), String(no));
  }

  private async deployResult(routeId: string, dto: DeployResultDto, etag: string | null): Promise<DeployResult> {
    this.remember(dto.route, etag);
    const route = RouteMapper.route(dto.route);
    const version = RouteMapper.version(routeId, dto.version, await this.versionDiff(routeId, dto.version.no));
    return { route, version, activity: RouteMapper.activity(route, version), deployment: RouteMapper.deployment(dto.deployment) };
  }
}
