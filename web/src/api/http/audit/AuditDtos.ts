import type { AuditEntry, AuditExportStatus, AuditFileFormat, AuditQuery } from '../../../domain';
import type { ApiSchemas } from '../../generated';
import { PageLimit, PageParam } from '../core/CursorIterator';
import type { Query } from '../core/HttpRequest';

/** `AuditEntry` of `GET /audit-log` (paths/access-control.yaml). */
export type AuditEntryDto = ApiSchemas['AuditEntry'];

/** `AuditAction` of `GET /audit-log/actions`. */
export type AuditActionDto = ApiSchemas['AuditAction'];

/** `AuditExportStatus` of `GET /audit-log/export/status`. */
export type AuditExportStatusDto = ApiSchemas['AuditExportStatus'];

/** REST paths and query keys of the audit log (relative to `/api/v1`). */
export const AuditEndpoint = {
  log: '/audit-log',
  actions: '/audit-log/actions',
  export: '/audit-log/export',
  exportStatus: '/audit-log/export/status',
  users: '/users',
  param: {
    actor: 'actor',
    text: 'q',
    sensitive: 'sensitive',
    action: 'action',
    objectType: 'objectType',
    teamId: 'teamId',
    from: 'from',
    to: 'to',
    format: 'format',
  },
  actionSeparator: ',',
} as const;

/** Maps audit DTOs to the domain: the server's resolved label is the object name, ids are kept for filters. */
export class AuditMapper {
  static entry(dto: AuditEntryDto): AuditEntry {
    return {
      id: dto.id,
      at: dto.at,
      actor: dto.actor.name || dto.actor.kind,
      actorId: dto.actor.id ?? null,
      action: dto.action,
      category: dto.category,
      object: dto.objectLabel || AuditMapper.fallbackObject(dto),
      objectType: dto.objectType,
      objectId: dto.objectId,
      ip: dto.ip,
      requestId: dto.requestId,
      sensitive: dto.sensitive,
      teamId: dto.teamId ?? null,
      diff: dto.diff ?? null,
    };
  }

  /** Objects without a name (sessions, deleted objects) show their type and id. */
  private static fallbackObject(dto: AuditEntryDto): string {
    return [dto.objectType.replace(/_/g, ' '), dto.objectId].filter(Boolean).join(' ');
  }

  /** The filter parameters shared by the log and the export (empty filters are left out). */
  static filters(q: AuditQuery): Query {
    const p = AuditEndpoint.param;
    const text = q.text.trim();
    return {
      [p.actor]: q.actorId || null,
      [p.text]: text || null,
      [p.sensitive]: q.sensitiveOnly ? true : null,
      [p.action]: q.actions.length ? q.actions.join(AuditEndpoint.actionSeparator) : null,
      [p.objectType]: q.objectType || null,
      [p.teamId]: q.teamId || null,
      [p.from]: q.from,
      [p.to]: q.to,
    };
  }

  static query(q: AuditQuery, cursor: string | null): Query {
    return { ...AuditMapper.filters(q), [PageParam.Limit]: PageLimit.Default, [PageParam.Cursor]: cursor };
  }

  static exportQuery(q: AuditQuery, format: AuditFileFormat): Query {
    return { ...AuditMapper.filters(q), [AuditEndpoint.param.format]: format };
  }

  static exportStatus(dto: AuditExportStatusDto): AuditExportStatus {
    return {
      enabled: dto.enabled,
      sink: dto.sink,
      target: dto.target,
      lagRows: dto.lagRows,
      exportedTotal: dto.checkpoint?.exportedTotal ?? 0,
      lastError: dto.checkpoint?.lastError || null,
      lastAttemptAt: dto.checkpoint?.lastAttemptAt ?? null,
    };
  }
}
