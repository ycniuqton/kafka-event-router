import { DiffMark, type DiffLine } from './version';

/** Why a version exists (server enum `route_version_origin`). */
export const VersionOrigin = { Deploy: 'deploy', Rollback: 'rollback', AutoRollback: 'auto_rollback', Import: 'import' } as const;
export type VersionOrigin = (typeof VersionOrigin)[keyof typeof VersionOrigin];

/** Deployment lifecycle states. */
export const DeploymentStatus = {
  PendingApproval: 'pending_approval',
  RollingOut: 'rolling_out',
  Active: 'active',
  Failed: 'failed',
  Superseded: 'superseded',
  Rejected: 'rejected',
  Cancelled: 'cancelled',
} as const;
export type DeploymentStatus = (typeof DeploymentStatus)[keyof typeof DeploymentStatus];

/** Which rules a validation run applies. */
export const ValidationMode = { Draft: 'draft', Deploy: 'deploy', Rollback: 'rollback', Import: 'import' } as const;
export type ValidationMode = (typeof ValidationMode)[keyof typeof ValidationMode];

export const IssueSeverity = { Error: 'error', Warning: 'warning' } as const;
export type IssueSeverity = (typeof IssueSeverity)[keyof typeof IssueSeverity];

/** One validation finding; `path` is a JSON Pointer into the route document. */
export type Issue = { code: string; severity: IssueSeverity; path: string; message: string };

export type ValidationResult = { valid: boolean; issues: Issue[] };

/** Sections of a route diff, in display order. */
export const DiffSection = {
  Source: 'source',
  Filter: 'filter',
  Transform: 'transform',
  Destination: 'destination',
  Settings: 'settings',
} as const;
export type DiffSection = (typeof DiffSection)[keyof typeof DiffSection];

export type RouteDiffLine = DiffLine & { path: string };
export type RouteDiffSection = { section: DiffSection; lines: RouteDiffLine[] };

/** Readable diff of two versions (number, `active` or `draft`). */
export type RouteDiff = { a: string; b: string; sections: RouteDiffSection[] };

/** Symbolic version references accepted by the diff endpoint. */
export const VersionRef = { Active: 'active', Draft: 'draft' } as const;

/** One deployment attempt of a version. */
export type Deployment = {
  id: string;
  versionNo: number;
  status: DeploymentStatus;
  failureReason: string;
  requestedAt: string;
  finishedAt: string | null;
};

/** Bulk actions on the routes list. */
export const BulkAction = { Pause: 'pause', Resume: 'resume', AddTag: 'add_tag' } as const;
export type BulkAction = (typeof BulkAction)[keyof typeof BulkAction];

export type BulkItemResult = { id: string; ok: boolean; message: string | null };

/** Rules over diffs and issues used by Review and History. */
export class RouteDiffRules {
  static lines(diff: RouteDiff): DiffLine[] {
    return diff.sections.flatMap((s) => s.lines.map((l) => ({ mark: l.mark, text: l.text })));
  }

  static changed(diff: RouteDiff): boolean {
    return diff.sections.some((s) => s.lines.length > 0);
  }

  static errors(issues: Issue[]): Issue[] {
    return issues.filter((i) => i.severity === IssueSeverity.Error);
  }

  static isMark(value: string): value is DiffMark {
    return (Object.values(DiffMark) as string[]).includes(value);
  }
}

/** One line of a side-by-side diff: what the older side had and what the newer side has. */
export type CompareRow = { mark: DiffMark; before?: string; after?: string };

/** One changed section of a diff (source, filter, transform, destination, settings) with its rows. */
export type CompareSection = { section: DiffSection; label: string; rows: CompareRow[] };

/** Readable, sectioned before/after view of a server `RouteDiff` (History compare and Review changes). */
export class RouteDiffPresenter {
  static readonly changeArrow = '→';

  static readonly sectionLabels: Record<DiffSection, string> = {
    [DiffSection.Source]: 'Source',
    [DiffSection.Filter]: 'Filter',
    [DiffSection.Transform]: 'Transform',
    [DiffSection.Destination]: 'Destination',
    [DiffSection.Settings]: 'Settings',
  };

  /** Sections that changed, each with its before/after rows. */
  static sections(diff: RouteDiff): CompareSection[] {
    return diff.sections
      .filter((s) => s.lines.length > 0)
      .map((s) => ({ section: s.section, label: RouteDiffPresenter.sectionLabels[s.section] ?? s.section, rows: s.lines.map((l) => RouteDiffPresenter.row(l)) }));
  }

  private static row(line: DiffLine): CompareRow {
    if (line.mark === DiffMark.Added) return { mark: line.mark, after: line.text };
    if (line.mark === DiffMark.Removed) return { mark: line.mark, before: line.text };
    const separator = ` ${RouteDiffPresenter.changeArrow} `;
    const at = line.text.indexOf(separator);
    if (at < 0) return { mark: line.mark, before: line.text, after: line.text };
    return { mark: line.mark, before: line.text.slice(0, at).trim(), after: line.text.slice(at + separator.length).trim() };
  }
}

/** Serialisations of exported route documents (connection names instead of ids). */
export const DocumentFormat = { Yaml: 'yaml', Json: 'json' } as const;
export type DocumentFormat = (typeof DocumentFormat)[keyof typeof DocumentFormat];

/** What an import does with a document whose route name already exists in the team. */
export const ImportMode = { Create: 'create', Upsert: 'upsert' } as const;
export type ImportMode = (typeof ImportMode)[keyof typeof ImportMode];

/** Per-document outcome of an import. */
export const ImportAction = {
  Created: 'created',
  DraftUpdated: 'draft_updated',
  SkippedExists: 'skipped_exists',
  Invalid: 'invalid',
  Failed: 'failed',
} as const;
export type ImportAction = (typeof ImportAction)[keyof typeof ImportAction];

/** An export ready to save: file name, media type and text. */
export type RouteExport = { fileName: string; mediaType: string; content: string };

export type ImportOptions = { mode: ImportMode; dryRun: boolean; format?: DocumentFormat };
export type ImportItem = { index: number; name: string; routeId: string | null; action: ImportAction; issues: Issue[] };
export type ImportReport = { dryRun: boolean; items: ImportItem[] };

/** Progress of a replay (server `replay_status`): the scheduler stops the runners, resets the group, restores. */
export const ReplayStatus = { Pending: 'pending', Stopping: 'stopping', Resetting: 'resetting', Done: 'done', Failed: 'failed' } as const;
export type ReplayStatus = (typeof ReplayStatus)[keyof typeof ReplayStatus];

/** One replay of a route (`POST /routes/{id}/replay`, `GET /routes/{id}/replays/{replayId}`). */
export type Replay = {
  id: string;
  routeId: string;
  status: ReplayStatus;
  from: string;
  requestedAt: string;
  startedAt: string | null;
  finishedAt: string | null;
  error: string;
  partitions: number;
};

/** A replay request: `from` is an ISO instant; `confirm` acknowledges that the destination receives duplicates. */
export type ReplayRequest = { from: string; confirm: boolean };

/** Labels, tones and the polling rule of replays. */
export class ReplayRules {
  static readonly pollIntervalMs = 2_000;

  static readonly statusLabels: Record<ReplayStatus, string> = {
    [ReplayStatus.Pending]: 'Queued',
    [ReplayStatus.Stopping]: 'Stopping runners',
    [ReplayStatus.Resetting]: 'Resetting offsets',
    [ReplayStatus.Done]: 'Replaying',
    [ReplayStatus.Failed]: 'Failed',
  };

  static finished(status: ReplayStatus): boolean {
    return status === ReplayStatus.Done || status === ReplayStatus.Failed;
  }
}
