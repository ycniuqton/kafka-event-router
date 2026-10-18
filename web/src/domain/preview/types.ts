import type { JsonValue } from '../json';

/** Sample sources of a preview session (mirror of Go `preview.SampleMode`). */
export const SampleMode = { Latest: 'latest', TimeRange: 'time_range', Upload: 'upload', Demo: 'demo' } as const;
export type SampleMode = (typeof SampleMode)[keyof typeof SampleMode];

/** Final outcome of one event in a preview run (mirror of Go `preview.EventStatus`). */
export const EventStatus = { Passed: 'passed', Filtered: 'filtered', Error: 'error', Dropped: 'dropped' } as const;
export type EventStatus = (typeof EventStatus)[keyof typeof EventStatus];

/** Why a sample load stopped (mirror of Go `kafka.StopReason`). */
export const StopReason = { Complete: 'complete', MaxEvents: 'max_events', MaxBytes: 'max_bytes', Deadline: 'deadline' } as const;
export type StopReason = (typeof StopReason)[keyof typeof StopReason];

/** Encodings of raw record text. */
export const RawEncoding = { Utf8: 'utf8', Base64: 'base64' } as const;
export type RawEncoding = (typeof RawEncoding)[keyof typeof RawEncoding];

/** Views of the sample panel. */
export const SampleView = { Pretty: 'pretty', Table: 'table', Raw: 'raw' } as const;
export type SampleView = (typeof SampleView)[keyof typeof SampleView];

/** RFC 6902 operations emitted by the preview trace (add / remove / replace only). */
export const PatchOpKind = { Add: 'add', Remove: 'remove', Replace: 'replace' } as const;
export type PatchOpKind = (typeof PatchOpKind)[keyof typeof PatchOpKind];

/** Run kinds: a draft preview, or the Test button (output validation mandatory). */
export const RunMode = { Preview: 'preview', Test: 'test' } as const;
export type RunMode = (typeof RunMode)[keyof typeof RunMode];

/** Preview limits and timings (mirror of Go `preview.SampleLimit` plus panel behaviour). */
export const PreviewLimit = {
  DefaultCount: 50,
  PeekCount: 20,
  MaxEvents: 1000,
  MaxUploadBytes: 10 * 1024 * 1024,
  PageSize: 100,
  DebounceMs: 300,
  BusyRetryMs: 1000,
} as const;

/** Value formats a session can be decoded with (mirror of Go `config.FormatType`); avro / protobuf use the schema registry. */
export const ValueFormat = { Json: 'json', String: 'string', Bytes: 'bytes', Avro: 'avro', Protobuf: 'protobuf' } as const;
export type ValueFormat = (typeof ValueFormat)[keyof typeof ValueFormat];

/** Texts the preview service writes instead of masked values (mirror of Go `preview.MaskToken`). */
export const MaskToken = { String: '••••••', Redacted: '[redacted]' } as const;
export type MaskToken = (typeof MaskToken)[keyof typeof MaskToken];

/** Failure kinds the sample panel reacts to differently (design §7). */
export const PreviewFailureKind = {
  SessionExpired: 'session_expired',
  InvalidConfig: 'invalid_config',
  Busy: 'busy',
  SourceUnavailable: 'source_unavailable',
  TopicNotFound: 'topic_not_found',
  Forbidden: 'forbidden',
  InvalidUpload: 'invalid_upload',
  Aborted: 'aborted',
  Other: 'other',
} as const;
export type PreviewFailureKind = (typeof PreviewFailureKind)[keyof typeof PreviewFailureKind];

/** `fieldMessage` is the first field error of a 422 (e.g. the JS compile error of the step at `path`). */
export type PreviewFailure = { kind: PreviewFailureKind; message: string; path?: string; fieldMessage?: string };

export type SampleHeader = { key: string; value: string };

/** One loaded record with its Kafka coordinates, decoded value and exact raw text. */
export type SampleRecord = {
  seq: number;
  topic: string;
  partition: number;
  offset: number;
  timestamp: string;
  key: JsonValue;
  value: JsonValue;
  headers: SampleHeader[];
  raw: { key: string | null; value: string; encoding: RawEncoding };
  decodeError: string | null;
};

export type SampleSession = {
  id: string;
  expiresAt: string;
  loaded: number;
  bytes: number;
  truncated: boolean;
  stopReason: StopReason;
  masked: boolean;
  /** Value format the session decodes with (absent on attached sessions). */
  format?: ValueFormat;
};

export type InlineEvent = { key?: JsonValue; value: JsonValue; headers?: SampleHeader[]; timestamp?: string; topic?: string };

export type LoadSamplesRequest = {
  connectionId?: string;
  topics?: string[];
  mode: SampleMode;
  count?: number;
  from?: string;
  to?: string;
  format?: string;
  events?: InlineEvent[];
  upload?: string;
};

export type SampleLoad = { session: SampleSession; events: SampleRecord[]; nextCursor?: string };
export type SamplePage = { events: SampleRecord[]; nextCursor?: string };
export type SampleQuery = { cursor?: string; limit?: number; q?: string };

export type ValueCount = { value: JsonValue; count: number; pct: number };

export type FieldNode = {
  path: string;
  types: Record<string, number>;
  present: number;
  frequency: number;
  topValues: ValueCount[];
  children: FieldNode[];
  /** Declared by the schema registry (avro / protobuf); frequency 0 when never seen in samples. */
  registry?: boolean;
};

export type FieldSchema = { samples: number; root: FieldNode };

/** `sampleSessionId`: the session inline `events` were copied from (Transform Studio); masking follows that session's source. */
export type RunRequest = {
  sessionId?: string;
  events?: InlineEvent[];
  sampleSessionId?: string;
  config: unknown;
  eventSeqs?: number[];
  writeToTopic?: boolean;
  routeId?: string;
};

export type PatchOp = { op: PatchOpKind; path: string; value?: JsonValue };

export type FilterNodeResult = { path: string; result: boolean; actual?: JsonValue; warning?: string | null };

export type StepTrace = { index: number; id: string; kind: string; patch: PatchOp[]; durationUs: number; logs: string[]; error: string | null };

export type RunError = { stepIndex: number | null; step?: string; path?: string; errorType: string; message: string };

export type PreviewOutput = {
  topic: string;
  key: JsonValue;
  value: JsonValue;
  headers: SampleHeader[];
  sizeBytes: number;
  valid: boolean;
  validationError: string | null;
};

export type BranchResult = {
  branch: string;
  status: EventStatus;
  filter: FilterNodeResult[];
  steps: StepTrace[];
  error: RunError | null;
  outputs: PreviewOutput[];
};

export type EventResult = { seq: number; status: EventStatus; error: RunError | null; branches: BranchResult[] };

export type RunSummary = {
  loaded: number;
  processed: number;
  passed: number;
  filtered: number;
  errors: number;
  dropped: number;
  incomplete: boolean;
  durationMs: number;
  /** Per-branch counts (FR-14), in document order. */
  branches?: BranchSummary[];
};

export type BranchSummary = { branch: string; passed: number; filtered: number; errors: number; dropped: number };

/** Temporary topic a test run wrote its passed outputs to (FR-22); deleted after `expiresAt`. */
export type TestTopic = { connectionId: string; topic: string; produced: number; expiresAt: string };

export type RunIssue = { path: string; severity: string; code?: string; message: string };

export type RunResult = { summary: RunSummary; issues: RunIssue[]; results: EventResult[]; masked?: boolean; testTopic?: TestTopic };

/** A value action from the panel's context menu (owners: routes filter, transform-studio steps). */
export const PanelValueActionKind = { KeepOnly: 'keep_only', Mask: 'mask', Rename: 'rename', Remove: 'remove', UseAsKey: 'use_as_key' } as const;
export type PanelValueActionKind = (typeof PanelValueActionKind)[keyof typeof PanelValueActionKind];

export type PanelValueAction = { kind: PanelValueActionKind; path: string; value: unknown };
