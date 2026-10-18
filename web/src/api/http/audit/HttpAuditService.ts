import type { AuditActionInfo, AuditActor, AuditExportStatus, AuditFileFormat, AuditPage, AuditQuery } from '../../../domain';
import { AuditService } from '../../contracts';
import { CursorIterator, PageLimit, type CursorPage } from '../core/CursorIterator';
import type { HttpContext } from '../core/HttpContext';
import { UrlBuilder } from '../core/HttpMessages';
import type { UserDto } from '../members/MemberDtos';
import { AuditEndpoint, AuditMapper, type AuditActionDto, type AuditEntryDto, type AuditExportStatusDto } from './AuditDtos';

/** `services.audit` over `GET /audit-log` (filters run on the server); actors come from `GET /users`. */
export class HttpAuditService extends AuditService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async query(query: AuditQuery, cursor: string | null): Promise<AuditPage> {
    const page = await this.context.api.get<CursorPage<AuditEntryDto>>(AuditEndpoint.log, AuditMapper.query(query, cursor));
    return { items: page.items.map(AuditMapper.entry), nextCursor: page.next_cursor };
  }

  async actors(): Promise<AuditActor[]> {
    const users = await new CursorIterator<UserDto>(this.context.api, AuditEndpoint.users, { limit: PageLimit.Max }).all();
    return users.map((u) => ({ id: u.id, name: u.displayName || u.email })).sort((a, b) => a.name.localeCompare(b.name));
  }

  async actions(): Promise<AuditActionInfo[]> {
    return (await this.context.api.get<{ items: AuditActionDto[] }>(AuditEndpoint.actions)).items;
  }

  /** A same-origin GET (session cookie, no CSRF needed): the browser streams the file to disk. */
  async exportUrl(query: AuditQuery, format: AuditFileFormat): Promise<string> {
    return UrlBuilder.build(this.context.api.baseUrl, AuditEndpoint.export, AuditMapper.exportQuery(query, format));
  }

  async exportStatus(): Promise<AuditExportStatus> {
    return AuditMapper.exportStatus(await this.context.api.get<AuditExportStatusDto>(AuditEndpoint.exportStatus));
  }
}
