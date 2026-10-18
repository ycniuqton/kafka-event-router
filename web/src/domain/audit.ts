/** Audit categories (Go `audit.Category`): changes, data access, administration. */
export const AuditCategory = { Change: 'change', Access: 'access', Admin: 'admin' } as const;
export type AuditCategory = (typeof AuditCategory)[keyof typeof AuditCategory];

/**
 * An append-only audit entry (design `AUDIT`); `sensitive` marks raw-data access (shown amber).
 * `actor` and `object` are display names; ids stay for filtering and links.
 */
export type AuditEntry = {
  id: string;
  at: string;
  actor: string;
  actorId: string | null;
  action: string;
  category: AuditCategory;
  object: string;
  objectType: string;
  objectId: string;
  ip: string;
  requestId: string;
  sensitive: boolean;
  teamId: string | null;
  /** JSON merge patch before → after (changes) or the access details (views, exports); null when none. */
  diff: unknown;
};

/**
 * Server-side audit filters (`GET /audit-log`): free text, one actor, actions, object type, team (empty = any),
 * an ISO time range (null = open) and "sensitive only".
 */
export type AuditQuery = {
  text: string;
  actorId: string;
  sensitiveOnly: boolean;
  actions: string[];
  objectType: string;
  teamId: string;
  from: string | null;
  to: string | null;
};

/** The unfiltered query. */
export const AnyAuditQuery: AuditQuery = { text: '', actorId: '', sensitiveOnly: false, actions: [], objectType: '', teamId: '', from: null, to: null };

/** One catalogued action (`GET /audit-log/actions`) offered by the filter bar. */
export type AuditActionInfo = { action: string; category: AuditCategory; sensitive: boolean };

/** Audited object types (Go `audit.ObjectType`) the filter bar offers. */
export const AuditObjectType = {
  Route: 'route',
  RouteVersion: 'route_version',
  Deployment: 'deployment',
  Connection: 'connection',
  Topic: 'topic',
  Transform: 'transform',
  LookupTable: 'lookup_table',
  SampleSession: 'sample_session',
  Dlq: 'dlq',
  User: 'user',
  Team: 'team',
  RoleBinding: 'role_binding',
  Grant: 'grant',
  Settings: 'settings',
  AuditLog: 'audit_log',
  Session: 'session',
  Secret: 'secret',
} as const;
export type AuditObjectType = (typeof AuditObjectType)[keyof typeof AuditObjectType];

/** Export file formats of `GET /audit-log/export`. */
export const AuditFileFormat = { Csv: 'csv', Jsonl: 'jsonl' } as const;
export type AuditFileFormat = (typeof AuditFileFormat)[keyof typeof AuditFileFormat];

/** Continuous SIEM export sinks (`ROUTER_AUDIT_EXPORT_SINK`). */
export const AuditExportSink = { None: 'none', Webhook: 'webhook', Syslog: 'syslog' } as const;
export type AuditExportSink = (typeof AuditExportSink)[keyof typeof AuditExportSink];

/** Progress of the continuous SIEM export (`GET /audit-log/export/status`). */
export type AuditExportStatus = {
  enabled: boolean;
  sink: AuditExportSink;
  target: string;
  lagRows: number;
  exportedTotal: number;
  lastError: string | null;
  lastAttemptAt: string | null;
};

/** One keyset page of the audit log; `nextCursor` is null on the last page. */
export type AuditPage = { items: AuditEntry[]; nextCursor: string | null };

/** A person the audit log can be filtered by. */
export type AuditActor = { id: string; name: string };

/** Display labels of audited actions; unknown keys are humanised ("route.rolled_back" → "Route rolled back"). */
export class AuditActions {
  private static readonly labels: Readonly<Record<string, string>> = {
    'auth.login': 'Signed in',
    'auth.logout': 'Signed out',
    'auth.login_failed': 'Sign-in failed',
    'route.created': 'Created route',
    'route.draft_saved': 'Saved route draft',
    'route.deployed': 'Deployed route',
    'route.paused': 'Paused route',
    'route.resumed': 'Resumed route',
    'route.rolled_back': 'Rolled back route',
    'route.archived': 'Archived route',
    'connection.created': 'Created connection',
    'connection.updated': 'Edited connection',
    'connection.deleted': 'Deleted connection',
    'connection.secret_replaced': 'Replaced credential',
    'connection.tested': 'Tested connection',
    'topic.created': 'Created topic',
    'transform.saved': 'Edited transform',
    'sample.viewed': 'Viewed samples',
    'sample.viewed_raw': 'Viewed raw (unmasked) samples',
    'preview.test_topic_written': 'Wrote test run to a test topic',
    'livetail.opened': 'Opened live tail',
    'dlq.retried': 'Retried dead letters',
    'dlq.discarded': 'Discarded dead letters',
    'dlq.exported': 'Exported dead letter',
    'role_binding.set': 'Changed role',
    'role_binding.removed': 'Removed from team',
    'user.updated': 'Updated user',
    'user.bootstrap_admin': 'Became first admin',
    'secret.read_by_service': 'Service read credential',
    'settings.updated': 'Changed workspace settings',
    'audit.exported': 'Exported audit log',
    'dlq.viewed': 'Viewed dead letter',
    'dlq.restored': 'Restored dead letters',
    'dlq.job_created': 'Started dead-letter background job',
    'dlq.job_cancelled': 'Cancelled dead-letter background job',
    'dlq.job_finished': 'Dead-letter background job ended',
    'deployment.approved': 'Approved deploy',
    'deployment.rejected': 'Rejected deploy',
    'deployment.withdrawn': 'Withdrew deploy request',
    'route.deploy_requested': 'Requested production deploy',
    'grant.added': 'Granted connection',
    'grant.removed': 'Revoked connection grant',
    'user.invited': 'Invited user',
  };

  static label(action: string): string {
    const known = AuditActions.labels[action];
    if (known) return known;
    const text = action.replace(/[._]/g, ' ').trim();
    return text.charAt(0).toUpperCase() + text.slice(1);
  }
}
