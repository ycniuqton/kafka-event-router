import {
  DeadLetterFileFormat,
  type DeadLetter,
  type DeadLetterActionReport,
  type DeadLetterDetail,
  type DeadLetterExportFile,
  type DeadLetterFilter,
  type DeadLetterJob,
  type DeadLetterJobAction,
  type DeadLetterPage,
  type DeadLetterSelection,
  type DeadLetterStudioLink,
  type DeadLetterSummary,
  type DeadLetterEdit,
  type DeadLetterGroupPage,
  type DeadLetterJobOptions,
  type DeadLetterOriginal,
  type DeadLetterReplayOptions,
  type DeadLetterReplayResult,
  type DryRunReport,
} from '../../../domain';
import { ApiError } from '../core/ApiError';
import { DeadLettersService } from '../../contracts';
import { PageParam } from '../core/CursorIterator';
import { HttpMethod } from '../core/HttpConstants';
import type { HttpContext } from '../core/HttpContext';
import type { Query } from '../core/HttpRequest';
import {
  DeadLetterAction,
  DeadLetterEndpoint,
  DeadLetterLimit,
  DeadLetterListSeparator,
  DeadLetterParam,
  type DeadLetterDetailDto,
  type DiscardReportDto,
  type GlobalSummaryDto,
  type JobDto,
  type RestoreReportDto,
  type ListPageDto,
  type RetryReportDto,
  type RouteSummaryDto,
  type StudioLinkDto,
  type DryRunReportDto,
  type GroupPageDto,
  type JobListDto,
  type ReplayResponseDto,
  ReplayApprovalRequiredCode,
} from './DeadLetterDtos';
import { DeadLetterMapper } from './DeadLetterMapper';
import { DeadLetterWorkflowMapper } from './DeadLetterWorkflowMapper';
import { DeadLetterJobStatus } from '../../../domain';

/** Export file conventions of the REST export. */
export const DeadLetterDownload = { fallbackName: 'dead-letters.json', header: 'Content-Disposition', dispositionName: /filename="([^"]+)"/ } as const;

/**
 * `services.deadLetters` over `/api/v1/routes/{id}/dlq*` and `/api/v1/dlq`. Ids are opaque per route, so the
 * service remembers each listed id's route to address retry / discard of a mixed selection.
 */
export class HttpDeadLettersService extends DeadLettersService {
  private readonly context: HttpContext;
  private readonly routeOf = new Map<string, string>();
  private readonly originals = new Map<string, DeadLetterOriginal | null>();

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async page(filter: DeadLetterFilter, cursor?: string | null): Promise<DeadLetterPage> {
    const path = filter.routeId ? DeadLetterEndpoint.route(filter.routeId) : DeadLetterEndpoint.global;
    const query = { ...DeadLetterMapper.query(filter), [PageParam.Limit]: DeadLetterLimit.PageSize, [PageParam.Cursor]: cursor ?? undefined };
    const dto = await this.context.api.get<ListPageDto>(path, query);
    dto.items.forEach((d) => this.routeOf.set(d.id, d.routeId));
    return DeadLetterMapper.page(dto);
  }

  async summary(routeId?: string): Promise<DeadLetterSummary> {
    if (!routeId) return DeadLetterMapper.summary(await this.context.api.get<GlobalSummaryDto>(DeadLetterEndpoint.summary()));
    return DeadLetterMapper.routeSummary(routeId, await this.context.api.get<RouteSummaryDto>(DeadLetterEndpoint.summary(routeId)));
  }

  async detail(deadLetter: DeadLetter): Promise<DeadLetterDetail> {
    const detail = DeadLetterMapper.detail(await this.context.api.get<DeadLetterDetailDto>(DeadLetterEndpoint.item(deadLetter.routeId, deadLetter.id)));
    this.originals.set(deadLetter.id, detail.original);
    return detail;
  }

  /** Per route; a route whose replay needs a second person (409 replay_approval_required) is reported, not sent. */
  async retry(ids: string[]): Promise<DeadLetterActionReport> {
    const reports = await Promise.all(this.byRoute(ids).map(([routeId, routeIds]) => this.retryRoute(routeId, routeIds)));
    return DeadLetterMapper.merge(reports);
  }

  private async retryRoute(routeId: string, ids: string[]): Promise<DeadLetterActionReport> {
    try {
      return DeadLetterMapper.retry(await this.context.api.post<RetryReportDto>(DeadLetterEndpoint.action(routeId, DeadLetterAction.Retry), { ids }));
    } catch (error) {
      if (!(error instanceof ApiError) || error.problem?.code !== ReplayApprovalRequiredCode) throw error;
      return { resolved: [], sent: 0, filtered: 0, failed: [], skipped: [], remaining: 0, approvalRequired: [{ routeId, ids }] };
    }
  }

  async retryMatching(routeId: string, filter: DeadLetterFilter): Promise<DeadLetterActionReport> {
    const body = { match: DeadLetterMapper.match(filter) };
    return DeadLetterMapper.retry(await this.context.api.post<RetryReportDto>(DeadLetterEndpoint.action(routeId, DeadLetterAction.Retry), body));
  }

  async discard(ids: string[]): Promise<DeadLetterActionReport> {
    const reports = await Promise.all(
      this.byRoute(ids).map(async ([routeId, routeIds]) =>
        DeadLetterMapper.discard(await this.context.api.post<DiscardReportDto>(DeadLetterEndpoint.action(routeId, DeadLetterAction.Discard), { ids: routeIds }), routeIds),
      ),
    );
    return DeadLetterMapper.merge(reports);
  }

  async exportJson(routeId: string, selection: DeadLetterSelection, format: DeadLetterFileFormat = DeadLetterFileFormat.Json): Promise<DeadLetterExportFile> {
    const base: Query = 'ids' in selection ? { [DeadLetterParam.Ids]: selection.ids.join(DeadLetterListSeparator) } : DeadLetterMapper.query(selection.filter);
    const query: Query = { ...base, [DeadLetterParam.Format]: format };
    const response = await this.context.api.raw({ method: HttpMethod.Get, path: DeadLetterEndpoint.action(routeId, DeadLetterAction.Export), query });
    const name = DeadLetterDownload.dispositionName.exec(response.headers.get(DeadLetterDownload.header) ?? '')?.[1];
    return { fileName: name ?? DeadLetterDownload.fallbackName, blob: await response.blob() };
  }

  async openInStudio(deadLetter: DeadLetter): Promise<DeadLetterStudioLink> {
    const link = await this.context.api.post<StudioLinkDto>(DeadLetterEndpoint.studio(deadLetter.routeId, deadLetter.id));
    return { routeId: deadLetter.routeId, sampleId: link.sampleIndex, sessionId: link.sessionId, builderUrl: link.builderUrl };
  }

  async restore(ids: string[]): Promise<DeadLetterActionReport> {
    const reports = await Promise.all(
      this.byRoute(ids).map(async ([routeId, routeIds]) =>
        DeadLetterMapper.restore(await this.context.api.post<RestoreReportDto>(DeadLetterEndpoint.action(routeId, DeadLetterAction.Restore), { ids: routeIds }), routeIds),
      ),
    );
    return DeadLetterMapper.merge(reports);
  }

  async startJob(routeId: string, action: DeadLetterJobAction, filter: DeadLetterFilter, options: DeadLetterJobOptions = {}): Promise<DeadLetterJob> {
    const selection = options.ids ? { ids: options.ids } : { match: DeadLetterMapper.match(filter) };
    const body = { action, ...selection, ratePerSecond: options.ratePerSecond, note: options.note || undefined };
    return DeadLetterMapper.job(await this.context.api.post<JobDto>(DeadLetterEndpoint.action(routeId, DeadLetterAction.Jobs), body));
  }

  async groups(filter: DeadLetterFilter): Promise<DeadLetterGroupPage> {
    const query = { ...DeadLetterMapper.query(filter), [DeadLetterParam.Fingerprint]: undefined };
    return DeadLetterWorkflowMapper.groups(await this.context.api.get<GroupPageDto>(DeadLetterEndpoint.groups(filter.routeId), query));
  }

  async replay(deadLetter: DeadLetter, edit: DeadLetterEdit | null, options: DeadLetterReplayOptions): Promise<DeadLetterReplayResult> {
    const original = edit ? await this.originalOf(deadLetter) : null;
    const body = DeadLetterWorkflowMapper.body(edit, original, options.dryRun, options.note);
    return DeadLetterWorkflowMapper.replay(await this.context.api.post<ReplayResponseDto>(DeadLetterEndpoint.replay(deadLetter.routeId, deadLetter.id), body));
  }

  async dryRun(routeId: string, selection: DeadLetterSelection): Promise<DryRunReport> {
    const body = 'ids' in selection ? { ids: selection.ids } : { match: DeadLetterMapper.match(selection.filter) };
    return DeadLetterWorkflowMapper.report(await this.context.api.post<DryRunReportDto>(DeadLetterEndpoint.action(routeId, DeadLetterAction.DryRun), body));
  }

  async approveJob(jobId: string, note: string): Promise<DeadLetterJob> {
    return DeadLetterMapper.job(await this.context.api.post<JobDto>(DeadLetterEndpoint.decideJob(jobId, DeadLetterAction.Approve), { note }));
  }

  async rejectJob(jobId: string, note: string): Promise<DeadLetterJob> {
    return DeadLetterMapper.job(await this.context.api.post<JobDto>(DeadLetterEndpoint.decideJob(jobId, DeadLetterAction.Reject), { note }));
  }

  async pendingApprovals(): Promise<DeadLetterJob[]> {
    const dto = await this.context.api.get<JobListDto>(DeadLetterEndpoint.jobs(), { [DeadLetterParam.Status]: DeadLetterJobStatus.PendingApproval });
    return dto.items.map((j) => DeadLetterMapper.job(j));
  }

  /** The original of an edited record (wire encodings), from the detail already shown or a fresh read. */
  private async originalOf(deadLetter: DeadLetter): Promise<DeadLetterOriginal | null> {
    if (!this.originals.has(deadLetter.id)) await this.detail(deadLetter);
    return this.originals.get(deadLetter.id) ?? null;
  }

  async job(jobId: string): Promise<DeadLetterJob> {
    return DeadLetterMapper.job(await this.context.api.get<JobDto>(DeadLetterEndpoint.job(jobId)));
  }

  async cancelJob(jobId: string): Promise<DeadLetterJob> {
    return DeadLetterMapper.job(await this.context.api.post<JobDto>(DeadLetterEndpoint.cancelJob(jobId)));
  }

  /** Groups ids by their route, in chunks the sync API accepts; unknown ids are dropped. */
  private byRoute(ids: string[]): [string, string[]][] {
    const groups = new Map<string, string[]>();
    ids.forEach((id) => {
      const routeId = this.routeOf.get(id);
      if (routeId) groups.set(routeId, [...(groups.get(routeId) ?? []), id]);
    });
    const out: [string, string[]][] = [];
    groups.forEach((routeIds, routeId) => {
      for (let i = 0; i < routeIds.length; i += DeadLetterLimit.SyncAction) out.push([routeId, routeIds.slice(i, i + DeadLetterLimit.SyncAction)]);
    });
    return out;
  }
}
