import {
  AuditActions,
  AuditCategory,
  AuditExportSink,
  AuditFileFormat,
  type AuditActionInfo,
  type AuditActor,
  type AuditEntry,
  type AuditExportStatus,
  type AuditPage,
  type AuditQuery,
} from '../../domain';
import { AuditService } from '../contracts';
import type { MockBackend } from './MockBackend';

/** Page size of the mock audit log (small so "Load more" is exercised). */
const MockAuditPaging = { pageSize: 5 } as const;

/** Sensitive catalog actions (Go `audit.NewCatalog`), used by the mock action list. */
const MockSensitiveActions: ReadonlySet<string> = new Set(['sample.viewed_raw', 'livetail.opened', 'dlq.exported', 'connection.secret_replaced', 'audit.exported', 'secret.read_by_service']);

/** In-memory audit log: the same filters as `GET /audit-log` and an offset cursor. */
export class MockAuditService extends AuditService {
  private readonly backend: MockBackend;

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  query(query: AuditQuery, cursor: string | null): Promise<AuditPage> {
    const matching = this.matching(query);
    const start = cursor ? Number(cursor) : 0;
    const end = start + MockAuditPaging.pageSize;
    return this.backend.reply({ items: matching.slice(start, end), nextCursor: end < matching.length ? String(end) : null });
  }

  actors(): Promise<AuditActor[]> {
    const byId = new Map<string, string>();
    this.backend.db.audit.forEach((e) => e.actorId && byId.set(e.actorId, e.actor));
    const actors = [...byId].map(([id, name]) => ({ id, name })).sort((a, b) => a.name.localeCompare(b.name));
    return this.backend.reply(actors);
  }

  actions(): Promise<AuditActionInfo[]> {
    const byAction = new Map<string, AuditCategory>();
    this.backend.db.audit.forEach((e) => byAction.set(e.action, e.category));
    const items = [...byAction].map(([action, category]) => ({ action, category, sensitive: MockSensitiveActions.has(action) }));
    return this.backend.reply(items.sort((a, b) => a.action.localeCompare(b.action)));
  }

  /** A blob URL of the filtered rows in the requested format (the real API streams the file). */
  exportUrl(query: AuditQuery, format: AuditFileFormat): Promise<string> {
    const body = MockAuditFile.render(this.matching(query), format);
    return this.backend.reply(URL.createObjectURL(new Blob([body], { type: MockAuditFile.mediaType[format] })));
  }

  exportStatus(): Promise<AuditExportStatus> {
    return this.backend.reply({ enabled: true, sink: AuditExportSink.Webhook, target: 'https://siem.vcc.internal/ingest', lagRows: 0,
      exportedTotal: this.backend.db.audit.length, lastError: null, lastAttemptAt: this.backend.db.audit[0]?.at ?? null });
  }

  private matching(query: AuditQuery): AuditEntry[] {
    return this.backend.db.audit.filter((e) => MockAuditMatcher.matches(e, query));
  }
}

/** The filter semantics of `GET /audit-log` over in-memory entries. */
export class MockAuditMatcher {
  static matches(e: AuditEntry, q: AuditQuery): boolean {
    const needle = q.text.trim().toLowerCase();
    const text = [e.object, e.objectId, e.action, AuditActions.label(e.action)].join(' ').toLowerCase();
    return (
      (!q.sensitiveOnly || e.sensitive) &&
      (!q.actorId || e.actorId === q.actorId) &&
      (!needle || text.includes(needle)) &&
      (!q.actions.length || q.actions.includes(e.action)) &&
      (!q.objectType || e.objectType === q.objectType) &&
      (!q.teamId || e.teamId === q.teamId) &&
      MockAuditMatcher.inRange(e.at, q)
    );
  }

  private static inRange(at: string, q: AuditQuery): boolean {
    const t = Date.parse(at);
    return (!q.from || t >= Date.parse(q.from)) && (!q.to || t < Date.parse(q.to));
  }
}

/** Renders mock export files with the server's columns. */
export class MockAuditFile {
  static readonly mediaType: Record<AuditFileFormat, string> = { [AuditFileFormat.Csv]: 'text/csv', [AuditFileFormat.Jsonl]: 'application/x-ndjson' };
  static readonly header = ['id', 'at', 'actor_id', 'actor_name', 'action', 'category', 'object_type', 'object_id', 'object_label', 'team_id', 'ip', 'request_id', 'sensitive'];

  static render(entries: AuditEntry[], format: AuditFileFormat): string {
    if (format === AuditFileFormat.Jsonl) return entries.map((e) => JSON.stringify(e)).join('\n') + '\n';
    const rows = entries.map((e) => [e.id, e.at, e.actorId ?? '', e.actor, e.action, e.category, e.objectType, e.objectId, e.object, e.teamId ?? '', e.ip, e.requestId, String(e.sensitive)]);
    return [MockAuditFile.header, ...rows].map((r) => r.map(MockAuditFile.cell).join(',')).join('\n') + '\n';
  }

  private static cell(value: string): string {
    return /[",\n]/.test(value) ? `"${value.replace(/"/g, '""')}"` : value;
  }
}
