import type {
  FieldNode,
  FieldSchema,
  FilterNodeResult,
  InlineEvent,
  PatchOp,
  RunIssue,
  RunRequest,
  RunResult,
  RunSummary,
  SampleHeader,
  SampleLoad,
  SamplePage,
  SampleQuery,
  SampleRecord,
  SampleSession,
  StepTrace,
} from '../../../domain/preview';

/** Wire enums and limits are the domain's (the REST shapes are identical). */
export { SampleMode, EventStatus as PreviewEventStatus, StopReason, RawEncoding, PreviewLimit } from '../../../domain/preview';
export type { LoadSamplesRequest, BranchResult as BranchResultDto, EventResult as EventResultDto, RunError as RunErrorDto } from '../../../domain/preview';
export type { PreviewOutput as OutputDto, ValueCount as ValueCountDto } from '../../../domain/preview';

/** Problem types specific to preview (RFC 7807 `type`). */
export const PreviewProblemType = {
  SessionExpired: 'https://eventrouter.dev/problems/preview/session-expired',
  InvalidConfig: 'https://eventrouter.dev/problems/preview/invalid-config',
  Busy: 'https://eventrouter.dev/problems/preview/busy',
  SourceUnavailable: 'https://eventrouter.dev/problems/preview/source-unavailable',
  TopicNotFound: 'https://eventrouter.dev/problems/preview/topic-not-found',
  InvalidUpload: 'https://eventrouter.dev/problems/preview/invalid-upload',
  Forbidden: 'https://eventrouter.dev/problems/preview/forbidden',
  TestTopicDenied: 'https://eventrouter.dev/problems/preview/test-topic-denied',
} as const;
export type PreviewProblemType = (typeof PreviewProblemType)[keyof typeof PreviewProblemType];

/** REST paths of live preview (relative to `/api/v1`). */
export class PreviewEndpoint {
  static readonly samples = '/preview/samples';
  static readonly run = '/preview/run';
  static readonly testRuns = '/preview/test-runs';

  static session(id: string): string {
    return `/preview/sessions/${encodeURIComponent(id)}`;
  }

  static events(id: string): string {
    return `${PreviewEndpoint.session(id)}/events`;
  }

  static schema(id: string): string {
    return `${PreviewEndpoint.session(id)}/schema`;
  }
}

/** Query keys of the events listing. */
export const PreviewQuery = { Cursor: 'cursor', Limit: 'limit', Text: 'q' } as const;

export type HeaderDto = SampleHeader;
export type InlineEventDto = InlineEvent;
export type SampleSessionDto = SampleSession;
export type SampleDto = SampleRecord;
export type LoadSamplesResponse = SampleLoad;
export type SamplesPageDto = SamplePage;
export type EventsQuery = SampleQuery;
export type FieldNodeDto = FieldNode;
export type FieldSchemaDto = FieldSchema;
export type RunRequestDto = RunRequest;
export type PatchOpDto = PatchOp;
export type FilterNodeResultDto = FilterNodeResult;
export type StepTraceDto = StepTrace;
export type RunSummaryDto = RunSummary;
export type RunIssueDto = RunIssue;
export type RunResultDto = RunResult;
