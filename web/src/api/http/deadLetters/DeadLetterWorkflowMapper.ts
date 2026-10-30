import {
  DeadLetterClasses,
  DeadLetterKind,
  DeadLetterReplayStatus,
  type DeadLetterEdit,
  type DeadLetterGroup,
  type DeadLetterGroupPage,
  type DeadLetterOriginal,
  type DeadLetterReplayResult,
  type DiffEntry,
  type DiffOp,
  type DryRunOutcome,
  type DryRunReport,
  type DryRunResult,
} from '../../../domain';
import {
  EncodingDto,
  type DiffEntryDto,
  type DryRunReportDto,
  type DryRunResultDto,
  type EncodedValueDto,
  type GroupDto,
  type GroupPageDto,
  type ReplayBodyDto,
  type ReplayResponseDto,
} from './DeadLetterDtos';
import { DeadLetterMapper } from './DeadLetterMapper';

/** Maps cause groups, dry runs and replays (dead-letters 00001-workflow §10) onto domain shapes. */
export class DeadLetterWorkflowMapper {
  static group(dto: GroupDto): DeadLetterGroup {
    const kind = DeadLetterMapper.kinds[dto.errorType] ?? DeadLetterKind.Unreadable;
    return {
      fingerprint: dto.fingerprint,
      routeId: dto.routeId,
      routeName: dto.routeName,
      stepId: dto.step,
      step: dto.stepLabel || dto.step,
      kind,
      errorClass: DeadLetterClasses.valid(dto.errorClass) ? dto.errorClass : DeadLetterClasses.ofKind(kind),
      pattern: dto.pattern,
      count: dto.count,
      firstSeenAt: dto.firstSeenAt,
      lastSeenAt: dto.lastSeenAt,
      versions: dto.versions ?? [],
      sample: dto.sample,
      trend: dto.trend,
    };
  }

  static groups(dto: GroupPageDto): DeadLetterGroupPage {
    const byClass = DeadLetterMapper.byClass(dto.byClass) ?? {};
    return { items: dto.items.map((g) => DeadLetterWorkflowMapper.group(g)), scanned: dto.scanned, truncated: dto.truncated, approximate: dto.approximate, byClass };
  }

  static result(dto: DryRunResultDto): DryRunResult {
    const outputs = (dto.outputs ?? []).map((o) => ({
      branch: o.branch,
      connectionId: o.connectionId,
      topic: o.topic,
      key: o.key?.data ?? null,
      value: o.value.data,
      headers: o.headers.map((h) => ({ key: h.key, value: h.value })),
    }));
    return { id: dto.id, outcome: dto.outcome as DryRunOutcome, errorType: dto.errorType, message: dto.message, outputs };
  }

  static report(dto: DryRunReportDto): DryRunReport {
    return { activeVersionNo: dto.activeVersionNo, sampled: dto.sampled, results: dto.results.map((r) => DeadLetterWorkflowMapper.result(r)), counts: dto.counts as DryRunReport['counts'] };
  }

  static diff(dto: DiffEntryDto[] | null): DiffEntry[] {
    return (dto ?? []).map((d) => ({ path: d.path, op: d.op as DiffOp }));
  }

  static replay(dto: ReplayResponseDto): DeadLetterReplayResult {
    const common = { diff: DeadLetterWorkflowMapper.diff(dto.diff), masked: !!dto.masked, activeVersionNo: dto.activeVersionNo };
    if (dto.dryRun) return { ...common, status: DeadLetterReplayStatus.Preview, preview: dto.result ? DeadLetterWorkflowMapper.result(dto.result) : undefined };
    if (dto.status === DeadLetterReplayStatus.PendingApproval) return { ...common, status: DeadLetterReplayStatus.PendingApproval, job: dto.job ? DeadLetterMapper.job(dto.job) : undefined };
    return { ...common, status: DeadLetterReplayStatus.Done, report: dto.report ? DeadLetterMapper.retry(dto.report) : undefined };
  }

  /** Request body: each edited member goes back in its original wire encoding (JSON trees stay JSON, text stays text). */
  static body(edit: DeadLetterEdit | null, original: DeadLetterOriginal | null, dryRun: boolean, note: string): ReplayBodyDto {
    if (!edit) return { dryRun, note };
    const out: NonNullable<ReplayBodyDto['edit']> = {};
    if (edit.key !== undefined) out.key = edit.key === null ? null : DeadLetterWorkflowMapper.encoded(edit.key, original?.keyEncoding);
    if (edit.value !== undefined) out.value = DeadLetterWorkflowMapper.encoded(edit.value, original?.valueEncoding);
    if (edit.headers) out.headers = edit.headers.map((h) => ({ key: h.key, value: h.value, encoding: EncodingDto.Utf8 }));
    return { edit: out, dryRun, note };
  }

  private static encoded(data: unknown, encoding: string | undefined): EncodedValueDto {
    const utf8 = encoding === EncodingDto.Utf8 && typeof data === 'string';
    return { encoding: utf8 ? EncodingDto.Utf8 : EncodingDto.Json, data };
  }
}
