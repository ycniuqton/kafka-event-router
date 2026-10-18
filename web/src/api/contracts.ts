import type {
  Activity,
  DemoSeed,
  OnboardingState,
  TailClientOp,
  TailFrame,
  Alert,
  AttentionItem,
  AuditActionInfo,
  AuditActor,
  AuditExportStatus,
  AuditFileFormat,
  AuditPage,
  AuditQuery,
  Clock,
  Connection,
  ConnectionDraft,
  ConnectionTestOptions,
  ConnectionTestRun,
  CurrentUser,
  DashboardStats,
  MetricsRange,
  RouteOverviewMetrics,
  DeadLetter,
  DeadLetterActionReport,
  DeadLetterDetail,
  DeadLetterExportFile,
  DeadLetterFileFormat,
  DeadLetterFilter,
  DeadLetterJob,
  DeadLetterJobAction,
  DeadLetterPage,
  DeadLetterSelection,
  DeadLetterStudioLink,
  DeadLetterSummary,
  LibraryLookupTable,
  LibraryTransform,
  Member,
  Route,
  RouteDraft,
  RouteTemplateId,
  RouteVersion,
  RouteDiff,
  Deployment,
  BulkAction,
  BulkItemResult,
  ValidationMode,
  ValidationResult,
  SampleEvent,
  TopicInfo,
  TopicQuery,
  TopicSettings,
} from '../domain';
import type { Access, RoleKey, TeamMember, UserPatch, WorkspaceSettings, WorkspaceSettingsPatch } from '../domain';
import type { Approval, ApprovalComment, ApprovalDetail, ApprovalFilter, ConnectionGrant, InviteDraft, Team, TeamDraft } from '../domain';
import type { TransformStep } from '../domain';
import type { InlineEvent, JsonValue } from '../domain';
import type { LibraryKind, LibraryQuery, LibraryUsage, LibraryVersion, LookupRowsInput, LookupRowsPage, NewLibraryTransform, NewLookupTable } from '../domain';
import type { NewTestCase, TestCase, TestCasePatch, TestCaseRunReport } from '../domain';
import type { RouteRuntime, WorkerInfo } from '../domain';
import type { Diagnostic, RuntimeInfo, SandboxEntry } from '../domain';
import type { RouteSettings, RouteSettingsPatch } from '../domain';
import type { AlertFeed, AlertRule, AlertRuleSave, ChannelTestResult, FlowMap, FlowMapQuery, PushEvent, PushState } from '../domain';
import type { ApiToken, ApiTokenDraft, CreatedApiToken } from '../domain';
import type { DocumentFormat, ImportOptions, ImportReport, RouteExport } from '../domain';
import type { Replay, ReplayRequest } from '../domain';
import type { FieldSchema, LoadSamplesRequest, PreviewFailure, RunRequest, RunResult, SampleLoad, SamplePage, SampleQuery, ValueFormat } from '../domain';

/** What a deploy returns: the updated route, its new version and the activity line it produced. */
export type DeployResult = { route: Route; version: RouteVersion; activity: Activity; deployment?: Deployment };

/** A field offered by the filter builder, from the sample schema. */
export type SampleField = { path: string; numeric: boolean };

/** Routes, drafts, deploys and version history (`/routes`). */
export abstract class RoutesService {
  abstract list(): Promise<Route[]>;
  abstract get(id: string): Promise<Route | undefined>;
  abstract pause(id: string): Promise<Route>;
  abstract resume(id: string): Promise<Route>;
  abstract duplicate(id: string): Promise<Route>;
  abstract archive(id: string): Promise<void>;
  abstract openDraft(routeId: string | null, templateId?: RouteTemplateId | null): Promise<RouteDraft>;
  abstract saveDraft(draft: RouteDraft): Promise<RouteDraft>;
  abstract deploy(draft: RouteDraft, note: string): Promise<DeployResult>;
  abstract listVersions(routeId: string): Promise<RouteVersion[]>;
  abstract rollback(routeId: string, version: number): Promise<RouteVersion>;
  /** Validates a draft (or the stored one when `draft` is null); issues carry JSON Pointer paths. */
  abstract validate(routeId: string, draft: RouteDraft | null, mode: ValidationMode): Promise<ValidationResult>;
  /** Readable diff of two version references (number, `active` or `draft`). */
  abstract diff(routeId: string, a: string, b: string): Promise<RouteDiff>;
  abstract listDeployments(routeId: string): Promise<Deployment[]>;
  /** Applies one action to many routes; every id reports its own result. */
  abstract bulk(action: BulkAction, ids: string[], tag?: string): Promise<BulkItemResult[]>;
  /** Settings tab: name, description, tags, replicas, plus error policy and limits of the draft. */
  abstract settings(routeId: string): Promise<RouteSettings>;
  /** Saves settings; error policy and limits go into the draft and apply on the next deploy. */
  abstract updateSettings(routeId: string, patch: RouteSettingsPatch): Promise<RouteSettings>;
  /** Archived routes (hidden from `list`), for the "Archived" status filter. */
  abstract listArchived(): Promise<Route[]>;
  /** Brings an archived route back as paused. */
  abstract unarchive(id: string): Promise<Route>;
  /** One or many routes as a YAML / JSON file (connection names instead of ids). */
  abstract exportRoutes(ids: string[], format: DocumentFormat, version?: string): Promise<RouteExport>;
  /** Imports YAML / JSON documents as drafts (never deploys); `dryRun` only reports what would happen. */
  abstract importRoutes(content: string, options: ImportOptions): Promise<ImportReport>;
  /** Re-reads the source from `request.from` (the destination receives duplicates); resolves with the pending replay. */
  abstract replay(routeId: string, request: ReplayRequest): Promise<Replay>;
  /** Progress of one replay (poll until `ReplayRules.finished`, or listen for `route.replay_changed`). */
  abstract replayStatus(routeId: string, replayId: string): Promise<Replay>;
}

/** Route runners and workers (`/routes/{id}/runtime`, `/workers`). */
export abstract class RuntimeService {
  /** Aggregated runtime of one route: observed state, running version, halt and per-replica status. */
  abstract get(routeId: string): Promise<RouteRuntime>;
  /** Data-plane workers (workspace admins). */
  abstract workers(): Promise<WorkerInfo[]>;
}

/** Saved clusters, connection tests and topics (`/connections`). */
export abstract class ConnectionsService {
  abstract list(): Promise<Connection[]>;
  /** One connection with its security details and the routes using it. */
  abstract get(id: string): Promise<Connection>;
  abstract create(draft: ConnectionDraft): Promise<Connection>;
  /** Saves an edit; empty secrets keep the stored ones. */
  abstract update(id: string, draft: ConnectionDraft): Promise<Connection>;
  /** Deletes an unused connection; rejects with `ConnectionInUseError` while routes use it. */
  abstract remove(id: string): Promise<void>;
  /** Runs the test checklist, reporting each progress snapshot; resolves with the final run. */
  abstract test(id: string, onUpdate: (run: ConnectionTestRun) => void, options?: ConnectionTestOptions): Promise<ConnectionTestRun>;
  /** Tests an unsaved draft (stored secrets of `connectionId` fill blanks); never changes saved health. */
  abstract testDraft(
    draft: ConnectionDraft,
    connectionId: string | null,
    onUpdate: (run: ConnectionTestRun) => void,
    options?: ConnectionTestOptions,
  ): Promise<ConnectionTestRun>;
  abstract listTopics(id: string, query?: TopicQuery): Promise<TopicInfo[]>;
  abstract createTopic(id: string, name: string, settings?: TopicSettings): Promise<TopicInfo>;
}

/** Failed events per route (`/routes/{id}/dlq`, `/dlq`). */
export abstract class DeadLettersService {
  /** One page of unresolved dead letters matching `filter`, newest first; pass the previous `nextCursor` to continue. */
  abstract page(filter: DeadLetterFilter, cursor?: string | null): Promise<DeadLetterPage>;
  /** Unresolved counts of every readable route (global badge, chips) or of one route. */
  abstract summary(routeId?: string): Promise<DeadLetterSummary>;
  /** Stack trace and original event of one dead letter. */
  abstract detail(deadLetter: DeadLetter): Promise<DeadLetterDetail>;
  /** Re-runs events through the route's active version; failed ones stay in the queue. */
  abstract retry(ids: string[]): Promise<DeadLetterActionReport>;
  abstract discard(ids: string[]): Promise<DeadLetterActionReport>;
  /** "Retry all": up to the sync cap of one route's events matching `filter`; `remaining` says how many are left. */
  abstract retryMatching(routeId: string, filter: DeadLetterFilter): Promise<DeadLetterActionReport>;
  /** Downloads the selected or matching events of one route as a JSON (default) or NDJSON file. */
  abstract exportJson(routeId: string, selection: DeadLetterSelection, format?: DeadLetterFileFormat): Promise<DeadLetterExportFile>;
  /** Creates a sample session with the failing event and returns the builder path. */
  abstract openInStudio(deadLetter: DeadLetter): Promise<DeadLetterStudioLink>;
  /** P2: un-resolves discarded events still inside DLQ retention. */
  abstract restore(ids: string[]): Promise<DeadLetterActionReport>;
  /** P2: queues a background retry/discard of every event of one route matching `filter` (any size). */
  abstract startJob(routeId: string, action: DeadLetterJobAction, filter: DeadLetterFilter): Promise<DeadLetterJob>;
  abstract job(jobId: string): Promise<DeadLetterJob>;
  abstract cancelJob(jobId: string): Promise<DeadLetterJob>;
}

export abstract class ActivityService {
  abstract list(): Promise<Activity[]>;
}

/** Audit log read side (`/audit-log`, workspace admins only): server-filtered keyset pages, newest first. */
export abstract class AuditService {
  abstract query(query: AuditQuery, cursor: string | null): Promise<AuditPage>;
  /** People the log can be filtered by. */
  abstract actors(): Promise<AuditActor[]>;
  /** The action catalog (`/audit-log/actions`) for the filter bar. */
  abstract actions(): Promise<AuditActionInfo[]>;
  /** A download URL of the filtered log (`/audit-log/export`, `audit.export`); the server streams the file. */
  abstract exportUrl(query: AuditQuery, format: AuditFileFormat): Promise<string>;
  /** Continuous SIEM export status (`/audit-log/export/status`). */
  abstract exportStatus(): Promise<AuditExportStatus>;
}

/** Transform library (`/transforms`, `/lookup-tables`): open read, writes to the caller's team. */
export abstract class LibraryService {
  abstract list(query?: LibraryQuery): Promise<LibraryTransform[]>;
  abstract lookupTables(query?: LibraryQuery): Promise<LibraryLookupTable[]>;
  abstract getTransform(id: string): Promise<LibraryTransform>;
  abstract getLookupTable(id: string): Promise<LibraryLookupTable>;
  /** Saves steps as a library transform: a new transform, or a new version when the name exists. */
  abstract saveTransform(name: string, steps: TransformStep[], options?: LibrarySaveOptions): Promise<LibraryTransform>;
  abstract createTransform(transform: NewLibraryTransform): Promise<LibraryTransform>;
  abstract publishVersion(id: string, steps: TransformStep[], changeNote: string): Promise<LibraryTransform>;
  abstract createLookupTable(table: NewLookupTable): Promise<LibraryLookupTable>;
  /** Publishes a new table version from grid rows or CSV text; per-line problems throw `LibraryImportError`. */
  abstract publishLookupRows(id: string, input: LookupRowsInput, changeNote: string): Promise<LibraryLookupTable>;
  abstract versions(kind: LibraryKind, id: string): Promise<LibraryVersion[]>;
  /** One version with its content (steps or the first rows). */
  abstract version(kind: LibraryKind, id: string, version: number): Promise<LibraryVersion>;
  abstract usages(kind: LibraryKind, id: string): Promise<LibraryUsage[]>;
  abstract rows(id: string, version: number, cursor?: string | null): Promise<LookupRowsPage>;
  /** Changes the description (If-Match on the object's row version). */
  abstract updateDescription<T extends LibraryTransform>(kind: LibraryKind, item: T, description: string): Promise<T>;
  /** Archives an object; refused while routes use it. */
  abstract archive(kind: LibraryKind, id: string): Promise<void>;
  /** Latest version per name, for "Update available" badges (unknown names are left out). */
  abstract latestVersions(kind: LibraryKind, names: string[]): Promise<Record<string, number>>;
  abstract testCases(transformId: string): Promise<TestCase[]>;
  abstract addTestCase(transformId: string, testCase: NewTestCase): Promise<TestCase>;
  abstract updateTestCase(testCase: TestCase, patch: TestCasePatch): Promise<TestCase>;
  abstract removeTestCase(testCase: TestCase): Promise<void>;
  /** Runs the transform's cases against `steps` (default: its latest version). */
  abstract runTestCases(transformId: string, steps?: TransformStep[], caseIds?: string[]): Promise<TestCaseRunReport>;
}

/** Optional metadata of "Save to library". */
export type LibrarySaveOptions = { description?: string; changeNote?: string };

/** Sample events for previews, topic peeks and the live tail (`/preview/samples`). */
export abstract class SamplesService {
  abstract list(connectionId: string, topic: string): Promise<SampleEvent[]>;
  abstract peek(connectionId: string, topic: string, count: number): Promise<SampleEvent[]>;
  abstract fields(): Promise<SampleField[]>;
}

/**
 * Live preview (`/preview/*`): sample sessions (latest / time range / upload), session pages and schema,
 * traced runs of a draft route config. `SamplesService.list/fields` are deprecated in its favour.
 */
export abstract class PreviewService {
  abstract loadSamples(request: LoadSamplesRequest): Promise<SampleLoad>;
  abstract listEvents(sessionId: string, query?: SampleQuery): Promise<SamplePage>;
  abstract schema(sessionId: string): Promise<FieldSchema>;
  abstract run(request: RunRequest, signal?: AbortSignal): Promise<RunResult>;
  abstract testRun(request: RunRequest, signal?: AbortSignal): Promise<RunResult>;
  abstract close(sessionId: string): Promise<void>;
  /** Classifies a rejected call so the panel can reload, retry or show the message (design §7). */
  abstract failure(error: unknown): PreviewFailure;
  /** Re-decodes the session with another value format (FR-8); returns the session and its first page. */
  reformat(sessionId: string, format: ValueFormat): Promise<SampleLoad> {
    return Promise.reject(new Error(`${PreviewService.unsupported}: ${sessionId} → ${format}`));
  }

  static readonly unsupported = 'Format override is not supported by this preview service';
}

export abstract class AlertsService {
  abstract list(): Promise<Alert[]>;
  abstract attention(): Promise<AttentionItem[]>;
  /** Bell feed (`GET /alerts`): newest alerts with the caller's unread count. */
  abstract feed(): Promise<AlertFeed>;
  /** Acknowledges one alert for the caller (`POST /alerts/{id}/ack`). */
  abstract ack(id: string): Promise<void>;
  /** Alert rules of a route (`GET /alert-rules?routeId=`). */
  abstract rules(routeId: string): Promise<AlertRule[]>;
  /** Creates, or updates with If-Match when `existing` is set; rejects with field errors (problem paths). */
  abstract saveRule(save: AlertRuleSave): Promise<AlertRule>;
  abstract deleteRule(id: string): Promise<void>;
  /** Sends a test notification to every channel of the rule. */
  abstract testRule(id: string): Promise<ChannelTestResult[]>;
}

/** Server push (`WS /events`): frames for route state, deployments, connection health and alerts. */
export abstract class PushEventsService {
  /** Starts the stream on first subscriber; returns the unsubscribe function. */
  abstract subscribe(listener: (event: PushEvent) => void): () => void;
  abstract onState(listener: (state: PushState) => void): () => void;
  abstract state(): PushState;
}

/** Callbacks of one live tail socket: frames, and the close code when the socket ends. */
export type TailHandlers = { onFrame: (frame: TailFrame) => void; onOpen?: () => void; onClose: (code: number) => void };

/** One open live tail socket: send pause/resume/filter ops, close it. */
export type TailChannel = { send(op: TailClientOp): boolean; close(): void };

/** Live tail (`WS /routes/{id}/tail`): one socket per open tab; reconnecting is the caller's job. */
export abstract class LiveTailService {
  abstract open(routeId: string, handlers: TailHandlers): TailChannel;
}

/** First-run state and demo data (`GET /onboarding`, `POST/DELETE /onboarding/demo`). */
export abstract class OnboardingService {
  abstract get(): Promise<OnboardingState>;
  /** Creates the team's demo connection; rejects with a 409 problem when it exists. */
  abstract createDemo(): Promise<DemoSeed>;
  abstract removeDemo(): Promise<void>;
}

/** Dashboard summary and route overview metrics (`/dashboard/summary`, `/routes/{id}/metrics`). */
export abstract class MetricsService {
  abstract dashboard(): Promise<DashboardStats>;
  abstract routeMetrics(routeId: string, range: MetricsRange): Promise<RouteOverviewMetrics>;
  /** Dashboard flow map (`GET /flow-map`): topic nodes and route-branch edges with events/sec. */
  abstract flowMap(query: FlowMapQuery): Promise<FlowMap>;
}

export abstract class MembersService {
  abstract list(): Promise<Member[]>;
  abstract current(): Promise<CurrentUser>;
}

/** Effective access and member administration (`/auth/me` access, `/users`, `/role-bindings`). */
export abstract class AccessService {
  abstract me(): Promise<Access>;
  /** One row per user and team binding (users without a team get one row with `teamId: null`). */
  abstract members(): Promise<TeamMember[]>;
  /** Creates or changes the member's role in their team (rejects with the server's message, e.g. last admin). */
  abstract setRole(member: TeamMember, role: RoleKey): Promise<TeamMember>;
  abstract removeBinding(member: TeamMember): Promise<void>;
  abstract updateUser(member: TeamMember, patch: UserPatch): Promise<void>;
  /** Pre-invites a user with one team role (`POST /users/invitations`); resolves with their member rows. */
  abstract invite(draft: InviteDraft): Promise<TeamMember[]>;
  /** Workspace-wide settings (`/workspace-settings`; changes need `settings.manage`). */
  abstract settings(): Promise<WorkspaceSettings>;
  abstract updateSettings(current: WorkspaceSettings, patch: WorkspaceSettingsPatch): Promise<WorkspaceSettings>;
}

/** Teams and their connection grants (`/teams`, `/teams/{id}/grants`). */
export abstract class TeamsService {
  abstract list(): Promise<Team[]>;
  abstract create(draft: TeamDraft): Promise<Team>;
  abstract update(team: Team, draft: TeamDraft): Promise<Team>;
  abstract remove(team: Team): Promise<void>;
  abstract grants(teamId: string): Promise<ConnectionGrant[]>;
  abstract addGrant(teamId: string, connectionId: string): Promise<ConnectionGrant>;
  abstract removeGrant(teamId: string, connectionId: string): Promise<void>;
}

/** Production deploy approvals (`/approvals`): list, review, approve / reject / withdraw, comments. */
export abstract class ApprovalsService {
  abstract list(filter: ApprovalFilter): Promise<Approval[]>;
  abstract get(id: string): Promise<ApprovalDetail>;
  abstract approve(approval: Approval, note: string): Promise<Approval>;
  abstract reject(approval: Approval, note: string): Promise<Approval>;
  abstract withdraw(approval: Approval): Promise<Approval>;
  abstract comment(approvalId: string, body: string): Promise<ApprovalComment>;
}

/** One step proposed by the assistant, with the diff line shown before accepting (confidence 0–1 when inferred). */
export type StepProposal = { text: string; step: TransformStep; confidence?: number; issue?: string };

/** One target of a transform-by-example proposal and where its value comes from. */
export type MappingProposal = { target: string; source?: string; reason: MappingReason; confidence: number };
export const MappingReason = { Value: 'value', Name: 'name', Type: 'type', Constant: 'constant' } as const;
export type MappingReason = (typeof MappingReason)[keyof typeof MappingReason];

/** Transform by example (`/preview/infer-mapping`): steps, per-target mappings and input paths that are dropped. */
export type InferredMapping = { proposals: StepProposal[]; mappings: MappingProposal[]; unmapped: string[] };

/** What the assistant may read: the studio's current steps, the sample's field paths (relative to `value`) and
 * at most two sample events (masked server-side when the connection is sensitive). */
export type AssistContext = { steps: TransformStep[]; fields: string[]; samples?: InlineEvent[]; connectionId?: string };

/** A plain-language explanation of one step or a chain. */
export type StepExplanation = { summary: string; bullets: string[] };

/** Transform Studio assistant (`/assist/*`): proposes recipe steps from words and explains steps (spec §8.6). */
export abstract class AssistService {
  abstract propose(instruction: string, context: AssistContext): Promise<StepProposal[]>;
  abstract explain(steps: TransformStep[]): Promise<StepExplanation>;
  /** Converts recipe steps into one equivalent JavaScript step (`/assist/export-js`); rejects unsupported kinds. */
  abstract exportJs(steps: TransformStep[]): Promise<TransformStep>;
  /** Proposes steps that turn `input` into the pasted `desired` value (transform by example). */
  abstract inferMapping(input: InlineEvent, desired: JsonValue): Promise<InferredMapping>;
}

/** The draft a test-case run compares against: the Studio's steps, run inside the route's saved draft (`caseIds` narrows the run). */
export type TestCaseDraft = { steps: TransformStep[]; caseIds?: string[] };

/** Saved test cases of a route (`/routes/{id}/test-cases`): pin, re-lock, delete, run against a draft. */
export abstract class TestCasesService {
  abstract list(routeId: string): Promise<TestCase[]>;
  abstract create(routeId: string, testCase: NewTestCase): Promise<TestCase>;
  /** Renames or re-locks a case (If-Match on its version). */
  abstract update(testCase: TestCase, patch: TestCasePatch): Promise<TestCase>;
  abstract remove(testCase: TestCase): Promise<void>;
  abstract run(routeId: string, draft: TestCaseDraft): Promise<TestCaseRunReport>;
}

/** JS sandbox facts for the Studio JS editor (`/sandbox/runtime-info`): limits, ctx docs, typings, rules. */
export abstract class SandboxService {
  abstract runtimeInfo(): Promise<RuntimeInfo>;
  /** Compiles code in the sandbox (`POST /sandbox/check`) and returns editor diagnostics; empty when it compiles. */
  abstract check(code: string, entry: SandboxEntry): Promise<Diagnostic[]>;
}

/** The caller's personal API tokens (`/auth/tokens`): list, create (secret shown once), revoke. */
export abstract class ApiTokensService {
  abstract list(): Promise<ApiToken[]>;
  abstract create(draft: ApiTokenDraft): Promise<CreatedApiToken>;
  abstract revoke(id: string): Promise<void>;
}

/** Everything screens may call. Obtain it only through `services` from `src/api`. */
export type Services = {
  routes: RoutesService;
  connections: ConnectionsService;
  deadLetters: DeadLettersService;
  activity: ActivityService;
  audit: AuditService;
  library: LibraryService;
  samples: SamplesService;
  preview: PreviewService;
  alerts: AlertsService;
  metrics: MetricsService;
  push: PushEventsService;
  liveTail: LiveTailService;
  onboarding: OnboardingService;
  members: MembersService;
  access: AccessService;
  teams: TeamsService;
  approvals: ApprovalsService;
  assist: AssistService;
  testCases: TestCasesService;
  runtime: RuntimeService;
  sandbox: SandboxService;
  apiTokens: ApiTokensService;
  clock: Clock;
};
