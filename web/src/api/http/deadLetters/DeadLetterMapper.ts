import {
  DeadLetterKind,
  type DeadLetterJob,
  type DeadLetterJobAction,
  type DeadLetterJobStatus,
  type DeadLetterResolution,
  type DeadLetterResolutionState,
  type DeadLetter,
  type DeadLetterActionReport,
  type DeadLetterDetail,
  type DeadLetterFilter,
  type DeadLetterKindCount,
  type DeadLetterPage,
  type DeadLetterSkipReason,
  type DeadLetterSummary,
  type DeliveryMode,
} from '../../../domain';
import { DeadLetterClasses, type DeadLetterErrorClass } from '../../../domain';
import type { Query } from '../core/HttpRequest';
import {
  DeadLetterErrorTypeDto,
  DeadLetterListSeparator,
  DeadLetterParam,
  EncodingDto,
  OutcomeDto,
  type ByErrorTypeDto,
  type DeadLetterDetailDto,
  type GlobalSummaryDto,
  type ListPageDto,
  type MatchDto,
  type RouteSummaryDto,
  type DeadLetterItemDto,
  type DiscardReportDto,
  type JobDto,
  type ResolutionDto,
  type RestoreReportDto,
  type ResultDto,
  type RetryReportDto,
} from './DeadLetterDtos';

/** Maps dead-letter DTOs onto the domain shapes the screens use. */
export class DeadLetterMapper {
  static readonly noSample = -1;

  static readonly kinds: Record<DeadLetterErrorTypeDto, DeadLetterKind> = {
    [DeadLetterErrorTypeDto.Decode]: DeadLetterKind.DecodeError,
    [DeadLetterErrorTypeDto.Step]: DeadLetterKind.TransformError,
    [DeadLetterErrorTypeDto.Timeout]: DeadLetterKind.Timeout,
    [DeadLetterErrorTypeDto.OutputInvalid]: DeadLetterKind.InvalidOutput,
    [DeadLetterErrorTypeDto.Unknown]: DeadLetterKind.Unreadable,
    [DeadLetterErrorTypeDto.LookupUnavailable]: DeadLetterKind.LookupUnavailable,
    [DeadLetterErrorTypeDto.DependencyUnavailable]: DeadLetterKind.DependencyUnavailable,
    [DeadLetterErrorTypeDto.ProduceRetriable]: DeadLetterKind.ProduceRetriable,
    [DeadLetterErrorTypeDto.Internal]: DeadLetterKind.InternalError,
    [DeadLetterErrorTypeDto.SandboxCrash]: DeadLetterKind.SandboxCrash,
    [DeadLetterErrorTypeDto.Encode]: DeadLetterKind.EncodeError,
    [DeadLetterErrorTypeDto.Produce]: DeadLetterKind.ProduceError,
    [DeadLetterErrorTypeDto.Config]: DeadLetterKind.ConfigError,
  };

  static item(dto: DeadLetterItemDto): DeadLetter {
    return {
      id: dto.id,
      failedAt: dto.failedAt,
      kind: DeadLetterMapper.kinds[dto.errorType] ?? DeadLetterKind.Unreadable,
      message: dto.message,
      stack: '',
      offset: dto.source.offset,
      partition: dto.source.partition,
      routeId: dto.routeId,
      step: dto.stepLabel || dto.step,
      sampleId: DeadLetterMapper.noSample,
      routeName: dto.routeName,
      versionNo: dto.versionNo,
      stepId: dto.step,
      resolution: DeadLetterMapper.resolution(dto.resolution),
      deliveryMode: (dto.deliveryMode || undefined) as DeliveryMode | undefined,
      errorClass: dto.errorClass && DeadLetterClasses.valid(dto.errorClass) ? dto.errorClass : undefined,
      fingerprint: dto.fingerprint || undefined,
      attempts: dto.attempts || undefined,
    };
  }

  static resolution(dto: ResolutionDto | null | undefined): DeadLetterResolution | null {
    return dto ? { state: dto.state as DeadLetterResolutionState, by: dto.by, at: dto.at, jobId: dto.jobId } : null;
  }

  static page(dto: ListPageDto): DeadLetterPage {
    return { items: dto.items.map((d) => DeadLetterMapper.item(d)), nextCursor: dto.next_cursor, truncated: dto.truncated };
  }

  /** Error types of the selected kinds (the unreadable kind maps to `unknown`). */
  static errorTypes(kinds: DeadLetterKind[]): DeadLetterErrorTypeDto[] {
    const entries = Object.entries(DeadLetterMapper.kinds) as [DeadLetterErrorTypeDto, DeadLetterKind][];
    return entries.filter(([, kind]) => kinds.includes(kind)).map(([type]) => type);
  }

  /** List / export query parameters of a filter (the route goes into the path, or `routeId` on `/dlq`). */
  static query(filter: DeadLetterFilter): Query {
    return {
      [DeadLetterParam.ErrorType]: DeadLetterMapper.errorTypes(filter.kinds).join(DeadLetterListSeparator) || undefined,
      [DeadLetterParam.Since]: filter.since,
      [DeadLetterParam.Until]: filter.until,
      [DeadLetterParam.Step]: filter.stepId || undefined,
      [DeadLetterParam.Version]: filter.versionNo ? String(filter.versionNo) : undefined,
      [DeadLetterParam.Text]: filter.text || undefined,
      [DeadLetterParam.IncludeResolved]: filter.includeResolved ? String(true) : undefined,
      [DeadLetterParam.Class]: filter.classes?.join(DeadLetterListSeparator) || undefined,
      [DeadLetterParam.Fingerprint]: filter.fingerprint || undefined,
    };
  }

  static match(filter: DeadLetterFilter): MatchDto {
    return {
      errorType: DeadLetterMapper.errorTypes(filter.kinds),
      since: filter.since,
      until: filter.until,
      step: filter.stepId || undefined,
      version: filter.versionNo || undefined,
      q: filter.text || undefined,
      fingerprint: filter.fingerprint || undefined,
      errorClass: filter.classes?.length ? filter.classes : undefined,
    };
  }

  static summary(dto: GlobalSummaryDto): DeadLetterSummary {
    const routes = dto.routes.map((r) => ({ routeId: r.routeId, routeName: r.routeName, unresolved: r.unresolved, byKind: DeadLetterMapper.byKind(r.byErrorType) }));
    return { unresolved: dto.unresolved, approximate: dto.approximate, byKind: DeadLetterMapper.byKind(dto.byErrorType), routes, byClass: DeadLetterMapper.byClass(dto.byErrorClass) };
  }

  static routeSummary(routeId: string, dto: RouteSummaryDto): DeadLetterSummary {
    const byKind = DeadLetterMapper.byKind(dto.byErrorType);
    const routes = dto.unresolved ? [{ routeId, routeName: '', unresolved: dto.unresolved, byKind }] : [];
    return { unresolved: dto.unresolved, approximate: dto.approximate, byKind, routes, byClass: DeadLetterMapper.byClass(dto.byErrorClass) };
  }

  /** Server class counts; absent (older server) → derived from the kinds by the screens. */
  static byClass(dto: Record<string, number> | null | undefined): Partial<Record<DeadLetterErrorClass, number>> | undefined {
    if (!dto) return undefined;
    return Object.fromEntries(Object.entries(dto).filter(([c]) => DeadLetterClasses.valid(c))) as Partial<Record<DeadLetterErrorClass, number>>;
  }

  private static byKind(dto: ByErrorTypeDto | null | undefined): DeadLetterKindCount[] {
    return (Object.entries(dto ?? {}) as [DeadLetterErrorTypeDto, number][])
      .filter(([, count]) => count > 0)
      .map(([type, count]) => ({ kind: DeadLetterMapper.kinds[type] ?? DeadLetterKind.Unreadable, count }));
  }

  static readonly schemaEncodings: string[] = [EncodingDto.Avro, EncodingDto.Protobuf];

  static detail(dto: DeadLetterDetailDto): DeadLetterDetail {
    const rawBytes = dto.value.encoding === EncodingDto.Base64 || dto.key.encoding === EncodingDto.Base64;
    const schema = DeadLetterMapper.schemaEncodings.includes(dto.value.encoding) ? { format: dto.value.encoding, schemaId: dto.value.schemaId } : undefined;
    const headers = dto.headers.map((h) => ({ key: h.key, value: h.value }));
    const encodings = { keyEncoding: dto.key.encoding, valueEncoding: dto.value.encoding };
    return { stack: dto.stack, original: { key: dto.key.data, value: dto.value.data, headers, rawBytes, masked: dto.masked, schema, ...encodings } };
  }

  static restore(dto: RestoreReportDto, ids: string[]): DeadLetterActionReport {
    const notRestored = new Set([...dto.skipped, ...(dto.failed ?? [])].map((r) => r.id));
    const restored = ids.filter((id) => !notRestored.has(id)).map((id) => ({ id, outcome: OutcomeDto.Restored }));
    return DeadLetterMapper.report([...restored, ...dto.skipped, ...(dto.failed ?? [])], dto.remaining, [OutcomeDto.Restored]);
  }

  static job(dto: JobDto): DeadLetterJob {
    return {
      id: dto.jobId,
      routeId: dto.routeId,
      routeName: dto.routeName,
      action: dto.action as DeadLetterJobAction,
      status: dto.status as DeadLetterJobStatus,
      total: dto.total,
      processed: dto.processed,
      counts: dto.counts ?? {},
      error: dto.error || null,
      createdAt: dto.createdAt,
      finishedAt: dto.finishedAt,
      ratePerSecond: dto.ratePerSecond,
      note: dto.note,
      edited: dto.edited,
      idsCount: dto.idsCount,
      requestedBy: dto.requestedBy,
      requestedByMe: dto.requestedByMe,
      approval: dto.approval,
    };
  }

  static retry(dto: RetryReportDto): DeadLetterActionReport {
    const report = DeadLetterMapper.report(dto.results, dto.remaining, [OutcomeDto.Sent, OutcomeDto.Filtered]);
    return dto.deliveredAtLeastOnce ? { ...report, deliveredAtLeastOnce: true } : report;
  }

  static discard(dto: DiscardReportDto, ids: string[]): DeadLetterActionReport {
    const notDiscarded = new Set([...dto.skipped, ...(dto.failed ?? [])].map((r) => r.id));
    const discarded = ids.filter((id) => !notDiscarded.has(id)).map((id) => ({ id, outcome: OutcomeDto.Discarded }));
    return DeadLetterMapper.report([...discarded, ...dto.skipped, ...(dto.failed ?? [])], dto.remaining, [OutcomeDto.Discarded]);
  }

  /** Merges several per-route reports into one. */
  static merge(reports: DeadLetterActionReport[]): DeadLetterActionReport {
    return reports.reduce(
      (all, r) => ({
        resolved: [...all.resolved, ...r.resolved],
        sent: all.sent + r.sent,
        filtered: all.filtered + r.filtered,
        failed: [...all.failed, ...r.failed],
        skipped: [...all.skipped, ...r.skipped],
        remaining: all.remaining + r.remaining,
        deliveredAtLeastOnce: all.deliveredAtLeastOnce || r.deliveredAtLeastOnce || undefined,
        approvalRequired: r.approvalRequired ? [...(all.approvalRequired ?? []), ...r.approvalRequired] : all.approvalRequired,
      }),
      { resolved: [], sent: 0, filtered: 0, failed: [], skipped: [], remaining: 0 } as DeadLetterActionReport,
    );
  }

  private static report(results: ResultDto[], remaining: number, resolvedOutcomes: string[]): DeadLetterActionReport {
    const of = (outcome: string) => results.filter((r) => r.outcome === outcome);
    return {
      resolved: results.filter((r) => resolvedOutcomes.includes(r.outcome)).map((r) => r.id),
      sent: of(OutcomeDto.Sent).length,
      filtered: of(OutcomeDto.Filtered).length,
      failed: of(OutcomeDto.Failed).map((r) => ({ id: r.id, message: r.message ?? '' })),
      skipped: of(OutcomeDto.Skipped).map((r) => ({ id: r.id, reason: r.reason as DeadLetterSkipReason })),
      remaining,
    };
  }
}
