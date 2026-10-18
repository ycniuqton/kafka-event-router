import type { LookupRows, Step } from '../../generated';
import {
  type ImportIssue,
  LibraryKind,
  LibraryRules,
  type JsonValue,
  type LibraryLookupTable,
  type LibraryTransform,
  type LibraryUsage,
  type LibraryVersion,
  type LookupKeyType,
  type LookupRowsPage,
} from '../../../domain';
import { StepMapper } from '../../../domain/routeConfig/StepMapper';

/** REST paths of the transform library (relative to `/api/v1`). */
export class LibraryEndpoint {
  static readonly transforms = '/transforms';
  static readonly lookupTables = '/lookup-tables';
  static readonly collections: Record<LibraryKind, string> = {
    [LibraryKind.Transform]: LibraryEndpoint.transforms,
    [LibraryKind.LookupTable]: LibraryEndpoint.lookupTables,
  };

  static item(kind: LibraryKind, id: string): string {
    return `${LibraryEndpoint.collections[kind]}/${encodeURIComponent(id)}`;
  }

  static versions(kind: LibraryKind, id: string): string {
    return `${LibraryEndpoint.item(kind, id)}/versions`;
  }

  static version(kind: LibraryKind, id: string, version: number): string {
    return `${LibraryEndpoint.versions(kind, id)}/${version}`;
  }

  static usages(kind: LibraryKind, id: string): string {
    return `${LibraryEndpoint.item(kind, id)}/usages`;
  }

  static rows(id: string, version: number): string {
    return `${LibraryEndpoint.version(LibraryKind.LookupTable, id, version)}/rows`;
  }

  static testCases(transformId: string): string {
    return `${LibraryEndpoint.item(LibraryKind.Transform, transformId)}/test-cases`;
  }

  static testCase(transformId: string, caseId: string): string {
    return `${LibraryEndpoint.testCases(transformId)}/${encodeURIComponent(caseId)}`;
  }

  static testRun(transformId: string): string {
    return `${LibraryEndpoint.testCases(transformId)}/run`;
  }
}

/** Query keys of library listings and imports. */
export const LibraryQueryKey = { Text: 'q', Names: 'names', Limit: 'limit', Cursor: 'cursor', ChangeNote: 'changeNote' } as const;

/** Media type of CSV imports and the paths of per-line import issues. */
export const LibraryImport = { csv: 'text/csv', linePrefix: '/lines/', listLimit: 200, namesSeparator: ',' } as const;

/** One library object as served by `GET /transforms` and `GET /lookup-tables`. */
export type LibraryItemDto = {
  id: string;
  kind: LibraryKind;
  name: string;
  description: string;
  keyType?: LookupKeyType;
  teamId: string;
  teamName: string;
  latestVersion: number;
  size: number;
  usageCount: number;
  archivedAt: string | null;
  rowVersion: number;
  updatedAt: string;
  updatedBy: string;
};

export type LibraryListDto = { items: LibraryItemDto[]; next_cursor?: string | null };

export type TransformWriteDto = { name?: string; description?: string; steps: Step[]; changeNote?: string };

export type LookupWriteDto = { name?: string; description?: string; keyType?: LookupKeyType; rows: LookupRows; changeNote?: string };

export type VersionDto = {
  version: number;
  size: number;
  configHash: string;
  changeNote: string;
  createdAt: string;
  createdBy: string;
  steps?: Step[];
  rows?: LookupRows;
};

export type UsageDto = LibraryUsage;

export type RowsDto = { items: LookupRows; total: number; next_cursor: string | null };

/** Maps library DTOs to the screens' domain types (names, version labels, team names). */
export class LibraryMapper {
  static transform(dto: LibraryItemDto): LibraryTransform {
    return {
      id: dto.id,
      name: dto.name,
      version: LibraryRules.versionLabel(dto.latestVersion),
      latestVersion: dto.latestVersion,
      description: dto.description,
      usedBy: dto.usageCount,
      team: dto.teamName,
      teamId: dto.teamId,
      size: dto.size,
      rowVersion: dto.rowVersion,
      archived: dto.archivedAt !== null,
      updatedAt: dto.updatedAt,
    };
  }

  static lookupTable(dto: LibraryItemDto): LibraryLookupTable {
    return { ...LibraryMapper.transform(dto), rows: dto.size, keyType: dto.keyType ?? 'string' };
  }

  static version(dto: VersionDto): LibraryVersion {
    const out: LibraryVersion = { version: dto.version, size: dto.size, changeNote: dto.changeNote, configHash: dto.configHash, createdAt: dto.createdAt, createdBy: dto.createdBy };
    if (dto.steps) out.steps = dto.steps.flatMap((s) => StepMapper.fromStep(s) ?? []);
    if (dto.rows) out.rows = dto.rows.map((r) => ({ key: r.key, value: r.value as JsonValue }));
    return out;
  }

  static rows(dto: RowsDto): LookupRowsPage {
    return { rows: dto.items.map((r) => ({ key: r.key, value: r.value as JsonValue })), total: dto.total, next: dto.next_cursor };
  }

  /** Per-line import issues of a 422 (`/lines/{n}`); empty when the problem is not an import refusal. */
  static importIssues(errors: { path: string; message: string }[]): ImportIssue[] {
    return errors
      .filter((e) => e.path.startsWith(LibraryImport.linePrefix))
      .map((e) => ({ line: Number(e.path.slice(LibraryImport.linePrefix.length)), message: e.message }));
  }
}
