/** Endpoints of the dead-letter API (relative to the API root). */
export class DeadLetterEndpoint {
  static readonly global = '/dlq';

  static route(routeId: string): string {
    return `/routes/${encodeURIComponent(routeId)}/dlq`;
  }

  static item(routeId: string, id: string): string {
    return `${DeadLetterEndpoint.route(routeId)}/${encodeURIComponent(id)}`;
  }

  static action(routeId: string, action: DeadLetterAction): string {
    return `${DeadLetterEndpoint.route(routeId)}/${action}`;
  }

  static studio(routeId: string, id: string): string {
    return `${DeadLetterEndpoint.item(routeId, id)}/${DeadLetterAction.Studio}`;
  }

  static job(jobId: string): string {
    return `${DeadLetterEndpoint.global}/${DeadLetterAction.Jobs}/${encodeURIComponent(jobId)}`;
  }

  static cancelJob(jobId: string): string {
    return `${DeadLetterEndpoint.job(jobId)}/${DeadLetterAction.Cancel}`;
  }

  static groups(routeId?: string): string {
    return `${routeId ? DeadLetterEndpoint.route(routeId) : DeadLetterEndpoint.global}/${DeadLetterAction.Groups}`;
  }

  static replay(routeId: string, id: string): string {
    return `${DeadLetterEndpoint.item(routeId, id)}/${DeadLetterAction.Replay}`;
  }

  static jobs(): string {
    return `${DeadLetterEndpoint.global}/${DeadLetterAction.Jobs}`;
  }

  static decideJob(jobId: string, decision: typeof DeadLetterAction.Approve | typeof DeadLetterAction.Reject): string {
    return `${DeadLetterEndpoint.job(jobId)}/${decision}`;
  }

  static summary(routeId?: string): string {
    return `${routeId ? DeadLetterEndpoint.route(routeId) : DeadLetterEndpoint.global}/${DeadLetterAction.Summary}`;
  }
}

/** Commands and sub-resources under `/routes/{id}/dlq`. */
export const DeadLetterAction = {
  Retry: 'retry',
  Discard: 'discard',
  Restore: 'restore',
  Export: 'export',
  Studio: 'studio',
  Summary: 'summary',
  Jobs: 'jobs',
  Cancel: 'cancel',
  Groups: 'groups',
  Replay: 'replay',
  DryRun: 'dry-run',
  Approve: 'approve',
  Reject: 'reject',
} as const;
export type DeadLetterAction = (typeof DeadLetterAction)[keyof typeof DeadLetterAction];

/** Query keys of the dead-letter endpoints. */
export const DeadLetterParam = {
  Ids: 'ids',
  Format: 'format',
  ErrorType: 'errorType',
  Since: 'since',
  Until: 'until',
  RouteId: 'routeId',
  Step: 'step',
  Version: 'version',
  Text: 'q',
  IncludeResolved: 'includeResolved',
  Class: 'class',
  Fingerprint: 'fingerprint',
  Status: 'status',
} as const;

/** Problem code of a sync retry that the strict 4-eyes gate turns into an approval request. */
export const ReplayApprovalRequiredCode = 'replay_approval_required';

/** Separator of multi-value query parameters. */
export const DeadLetterListSeparator = ',';

/** Limits of the REST API (design §2.2). */
export const DeadLetterLimit = { PageSize: 50, SyncAction: 500 } as const;

/** Error types written by the worker (`router.error_type`). */
export const DeadLetterErrorTypeDto = {
  Decode: 'decode_error',
  Step: 'step_error',
  Timeout: 'timeout',
  OutputInvalid: 'output_invalid',
  Unknown: 'unknown',
  LookupUnavailable: 'lookup_unavailable',
  DependencyUnavailable: 'dependency_unavailable',
  ProduceRetriable: 'produce_retriable',
  Internal: 'internal_error',
  SandboxCrash: 'sandbox_crash',
  Encode: 'encode_error',
  Produce: 'produce_error',
  Config: 'config_error',
} as const;
export type DeadLetterErrorTypeDto = (typeof DeadLetterErrorTypeDto)[keyof typeof DeadLetterErrorTypeDto];

/** Value encodings of detail payloads. */
export const EncodingDto = { Json: 'json', Utf8: 'utf8', Base64: 'base64', Avro: 'avro', Protobuf: 'protobuf' } as const;
export type EncodingDto = (typeof EncodingDto)[keyof typeof EncodingDto];

export type SourcePositionDto = { topic: string; partition: number; offset: number };

export type DeadLetterItemDto = {
  id: string;
  routeId: string;
  routeName: string;
  failedAt: string;
  errorType: DeadLetterErrorTypeDto;
  message: string;
  step: string;
  stepLabel?: string;
  branch: string;
  versionNo: number;
  source: SourcePositionDto;
  resolution: ResolutionDto | null;
  deliveryMode?: string;
  errorClass?: string;
  fingerprint?: string;
  attempts?: number;
};

export type ResolutionDto = { state: string; by: string; at: string; jobId?: string };

export type EncodedValueDto = { encoding: EncodingDto; data: unknown; schemaId?: number };

export type DeadLetterDetailDto = DeadLetterItemDto & {
  stack: string;
  key: EncodedValueDto;
  value: EncodedValueDto;
  headers: { key: string; value: string; encoding: EncodingDto }[];
  masked: boolean;
};

export type ResultDto = { id: string; outcome: string; errorType?: string; message?: string; reason?: string };

export type RetryReportDto = {
  activeVersionNo: number;
  results: ResultDto[];
  counts: Record<string, number>;
  remaining: number;
  deliveryMode?: string;
  deliveredAtLeastOnce?: boolean;
};

export type DiscardReportDto = { discarded: number; skipped: ResultDto[]; failed?: ResultDto[]; remaining: number };

export type RestoreReportDto = { restored: number; skipped: ResultDto[]; failed?: ResultDto[]; remaining: number };

export type JobDto = {
  jobId: string;
  routeId: string;
  routeName: string;
  action: string;
  status: string;
  total: number;
  processed: number;
  counts: Record<string, number> | null;
  error?: string;
  createdAt: string;
  finishedAt: string | null;
  ratePerSecond?: number;
  note?: string;
  edited?: boolean;
  idsCount?: number;
  requestedBy?: string;
  requestedByMe?: boolean;
  approval?: { required: boolean; decidedBy?: string; decidedAt?: string; decisionNote?: string };
};

export type JobListDto = { items: JobDto[] };

export type DryRunOutputDto = { branch: string; connectionId: string; topic: string; key: EncodedValueDto | null; value: EncodedValueDto; headers: { key: string; value: string; encoding?: EncodingDto }[] };

export type DryRunResultDto = { id: string; outcome: string; errorType?: string; message?: string; outputs: DryRunOutputDto[] | null };

export type DiffEntryDto = { path: string; op: string };

export type ReplayResponseDto = {
  dryRun: boolean;
  status?: string;
  activeVersionNo?: number;
  masked?: boolean;
  diff: DiffEntryDto[] | null;
  result?: DryRunResultDto;
  report?: RetryReportDto;
  job?: JobDto;
};

export type DryRunReportDto = { activeVersionNo: number; sampled: number; results: DryRunResultDto[]; counts: Record<string, number> };

/** Body of `POST …/{dlqId}/replay`; omitted members keep the original. */
export type ReplayBodyDto = {
  edit?: { key?: EncodedValueDto | null; value?: EncodedValueDto; headers?: { key: string; value: string; encoding: EncodingDto }[] };
  dryRun: boolean;
  note: string;
};

export type GroupDto = {
  fingerprint: string;
  routeId: string;
  routeName: string;
  step: string;
  stepLabel?: string;
  errorType: DeadLetterErrorTypeDto;
  errorClass: string;
  pattern: string;
  count: number;
  firstSeenAt: string;
  lastSeenAt: string;
  versions: number[] | null;
  sample: { id: string; message: string; failedAt: string };
  trend: { since: string; bucketSeconds: number; counts: number[] };
};

export type GroupPageDto = { items: GroupDto[]; scanned: number; truncated: boolean; approximate: boolean; byClass: Record<string, number> | null };

export type StudioLinkDto = { sessionId: string; sampleIndex: number; builderUrl: string };

/** Outcomes of per-event results. */
export const OutcomeDto = { Sent: 'sent', Filtered: 'filtered', Failed: 'failed', Skipped: 'skipped', Discarded: 'discarded', Restored: 'restored' } as const;

/** Body filter of a match selection (`POST …/retry {match}`). */
export type MatchDto = {
  errorType: DeadLetterErrorTypeDto[];
  since?: string;
  until?: string;
  step?: string;
  version?: number;
  q?: string;
  fingerprint?: string;
  errorClass?: string[];
};

export type ListPageDto = { items: DeadLetterItemDto[]; next_cursor: string | null; scanned: number; truncated: boolean };

export type ByErrorTypeDto = Partial<Record<DeadLetterErrorTypeDto, number>>;

export type RouteSummaryDto = { unresolved: number; byErrorType: ByErrorTypeDto | null; approximate: boolean; byErrorClass?: Record<string, number> | null };

export type GlobalSummaryDto = RouteSummaryDto & {
  routes: (RouteSummaryDto & { routeId: string; routeName: string })[];
};
