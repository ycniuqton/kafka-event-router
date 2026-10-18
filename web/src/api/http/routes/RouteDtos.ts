import type { RouteConfig } from '../../generated';
import type { DeploymentStatus, ImportAction, Issue, ReplayStatus, RouteDiff, RouteStatus, VersionOrigin } from '../../../domain';

/** Endpoints of `/api/v1/routes` (relative to the API root). */
export class RouteEndpoint {
  static readonly collection = '/routes';
  static readonly bulkPath = '/routes/bulk';
  static readonly exportPath = '/routes/export';
  static readonly importPath = '/routes/import';

  static item(id: string): string {
    return `${RouteEndpoint.collection}/${encodeURIComponent(id)}`;
  }

  static action(id: string, action: RouteAction): string {
    return `${RouteEndpoint.item(id)}/${action}`;
  }

  static version(id: string, no: number): string {
    return `${RouteEndpoint.action(id, RouteAction.Versions)}/${no}`;
  }

  static replay(id: string, replayId: string): string {
    return `${RouteEndpoint.action(id, RouteAction.Replays)}/${encodeURIComponent(replayId)}`;
  }

  static diff(id: string, a: string, b: string): string {
    return `${RouteEndpoint.action(id, RouteAction.Versions)}/${encodeURIComponent(a)}/diff/${encodeURIComponent(b)}`;
  }
}

/** Sub-resources and commands of one route. */
export const RouteAction = {
  Duplicate: 'duplicate',
  Validate: 'validate',
  Deploy: 'deploy',
  Pause: 'pause',
  Resume: 'resume',
  Rollback: 'rollback',
  Versions: 'versions',
  Deployments: 'deployments',
  Unarchive: 'unarchive',
  Export: 'export',
  Replay: 'replay',
  Replays: 'replays',
} as const;
export type RouteAction = (typeof RouteAction)[keyof typeof RouteAction];

export type UserRefDto = { id: string; name: string } | null;

export type RouteSummaryDto = {
  id: string;
  name: string;
  teamId: string;
  teamName: string;
  tags: string[];
  status: RouteStatus;
  activeVersion: { id: string; no: number } | null;
  hasUndeployedDraft: boolean;
  source: {
    connectionId: string;
    connectionName: string;
    topics: string[];
  } | null;
  filterSummary: { headline: string; more: number } | null;
  transform: 'none' | 'visual' | 'js';
  destinations: {
    branchId: string;
    connectionId: string;
    connectionName: string;
    topic: string;
  }[];
  branches?: {
    id: string;
    filterSummary: { headline: string; more: number } | null;
    transform: 'none' | 'visual' | 'js';
  }[];
  metrics: {
    eventsPerSec: number;
    matchPct: number;
    lag: number;
    sparkline: number[];
  } | null;
  updatedAt: string;
  updatedBy: UserRefDto;
};

export type DeploymentDto = {
  id: string;
  versionNo: number;
  status: DeploymentStatus;
  failureReason?: string;
  requestedAt: string;
  finishedAt?: string;
};

export type RouteDetailDto = RouteSummaryDto & {
  description: string;
  replicas: number;
  revision: number;
  draft: RouteConfig | null;
  active: RouteConfig | null;
  lastDeployment: DeploymentDto | null;
  issues?: Issue[];
};

export type RouteVersionDto = {
  id: string;
  no: number;
  origin: VersionOrigin;
  basedOn?: number;
  author: UserRefDto;
  changeNote: string;
  createdAt: string;
};

export type VersionSummaryDto = Omit<RouteVersionDto, 'id'> & {
  active: boolean;
  configHash: string;
  deploymentStatus?: DeploymentStatus;
};

export type DeployResultDto = {
  deployment: DeploymentDto;
  version: RouteVersionDto;
  route: RouteDetailDto;
};

export type RouteDiffDto = RouteDiff;

export type BulkResultDto = {
  results: {
    id: string;
    ok: boolean;
    problem?: { detail?: string; title: string };
  }[];
};

/** Query keys of the routes collection and import endpoints. */
export const RouteQuery = {
  Status: 'status',
  Format: 'format',
  Version: 'version',
  Mode: 'mode',
  DryRun: 'dryRun',
} as const;

export type ImportResultDto = {
  index: number;
  name: string;
  routeId?: string;
  action: ImportAction;
  issues: Issue[];
};
export type ImportReportDto = { dryRun: boolean; results: ImportResultDto[] };

/** A replay as the API returns it (P3). */
export type ReplayDto = {
  replayId: string;
  routeId: string;
  status: ReplayStatus;
  from: string;
  requestedAt: string;
  startedAt?: string;
  finishedAt?: string;
  error?: string;
  partitions: number;
};
