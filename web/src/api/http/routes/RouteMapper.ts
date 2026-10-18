import {
  ActivityVerb,
  RouteDiffRules,
  TransformMode,
  type Activity,
  type Deployment,
  type Replay,
  type Route,
  type RouteDiff,
  type RouteMetrics,
  type RouteVersion,
} from '../../../domain';
import type { DeploymentDto, ReplayDto, RouteSummaryDto, RouteVersionDto, UserRefDto, VersionSummaryDto } from './RouteDtos';

/** Texts the mapper fills in when the server has none. */
export const RouteMapperText = {
  allEvents: 'all events',
  unknownAuthor: 'system',
  noNote: '—',
  firstVersion: 'First version',
  versionPrefix: 'v',
} as const;

const transformModes: Record<RouteSummaryDto['transform'], TransformMode> = {
  none: TransformMode.None,
  visual: TransformMode.Recipe,
  js: TransformMode.Js,
};

/** Maps routes DTOs to the UI's domain types. */
export class RouteMapper {
  static route(dto: RouteSummaryDto): Route {
    const destination = dto.destinations[0];
    return {
      id: dto.id,
      name: dto.name,
      status: dto.status,
      owner: dto.teamName,
      source: {
        connectionId: dto.source?.connectionId ?? '',
        connectionName: dto.source?.connectionName,
        topics: dto.source?.topics ?? [],
      },
      destination: {
        connectionId: destination?.connectionId ?? '',
        connectionName: destination?.connectionName,
        topic: destination?.topic ?? '',
      },
      filterSummary: RouteMapper.filterSummary(dto.filterSummary),
      transformMode: transformModes[dto.transform] ?? TransformMode.None,
      metrics: RouteMapper.metrics(dto.metrics),
      updatedAt: dto.updatedAt,
      version: dto.activeVersion?.no ?? 0,
      tags: dto.tags ?? [],
      updatedBy: dto.updatedBy?.name,
      hasUndeployedDraft: dto.hasUndeployedDraft,
    };
  }

  /** Server metrics, or zeros flagged `unavailable` when the metrics backend has none. */
  static metrics(dto: RouteSummaryDto['metrics']): RouteMetrics {
    if (!dto) return { eventsPerSec: 0, matchPct: 0, lag: 0, unavailable: true };
    return {
      eventsPerSec: dto.eventsPerSec,
      matchPct: dto.matchPct,
      lag: dto.lag,
      sparkline: dto.sparkline,
    };
  }

  static filterSummary(f: RouteSummaryDto['filterSummary']): string {
    if (!f) return RouteMapperText.allEvents;
    return f.more > 0 ? `${f.headline} +${f.more}` : f.headline;
  }

  static version(routeId: string, dto: RouteVersionDto | VersionSummaryDto, diff: RouteDiff | null): RouteVersion {
    const lines = diff ? RouteDiffRules.lines(diff) : [];
    const summary = dto as Partial<VersionSummaryDto>;
    return {
      routeId,
      version: dto.no,
      author: RouteMapper.author(dto.author),
      createdAt: dto.createdAt,
      note: dto.changeNote || RouteMapperText.noNote,
      diff: lines,
      origin: dto.origin,
      basedOn: dto.basedOn,
      active: summary.active,
      deploymentStatus: summary.deploymentStatus,
    };
  }

  static author(ref: UserRefDto): string {
    return ref?.name || RouteMapperText.unknownAuthor;
  }

  static deployment(dto: DeploymentDto): Deployment {
    return {
      id: dto.id,
      versionNo: dto.versionNo,
      status: dto.status,
      failureReason: dto.failureReason ?? '',
      requestedAt: dto.requestedAt,
      finishedAt: dto.finishedAt ?? null,
    };
  }

  static replay(dto: ReplayDto): Replay {
    return {
      id: dto.replayId,
      routeId: dto.routeId,
      status: dto.status,
      from: dto.from,
      requestedAt: dto.requestedAt,
      startedAt: dto.startedAt ?? null,
      finishedAt: dto.finishedAt ?? null,
      error: dto.error ?? '',
      partitions: dto.partitions,
    };
  }

  static activity(route: Route, version: RouteVersion): Activity {
    return {
      id: `deploy-${route.id}-${version.version}`,
      actor: version.author,
      verb: ActivityVerb.Deployed,
      routeName: route.name,
      detail: RouteMapperText.versionPrefix + version.version,
      at: version.createdAt,
    };
  }
}
