import { Tone } from './tone';
import type { DeploymentStatus, VersionOrigin } from './routeLifecycle';

/** Kind of change in a version diff line. */
export const DiffMark = { Added: '+', Removed: '-', Changed: '~' } as const;
export type DiffMark = (typeof DiffMark)[keyof typeof DiffMark];

export type DiffLine = { mark: DiffMark; text: string };

/** An immutable deployed version of a route (design `VERS`). */
export type RouteVersion = {
  routeId: string;
  version: number;
  author: string;
  createdAt: string;
  note: string;
  diff: DiffLine[];
  /** Why the version exists and which version it restored (REST only). */
  origin?: VersionOrigin;
  basedOn?: number;
  /** True for the version the route currently runs; status of its deployment. */
  active?: boolean;
  deploymentStatus?: DeploymentStatus;
};

export const DiffMarkTone: Record<DiffMark, Tone> = {
  [DiffMark.Added]: Tone.Ok,
  [DiffMark.Removed]: Tone.Err,
  [DiffMark.Changed]: Tone.Warn,
};

/** Diff rules used by rollback: a rollback inverts adds and removes. */
export class VersionDiff {
  static invert(diff: DiffLine[]): DiffLine[] {
    return diff.map((line) => ({ mark: VersionDiff.invertMark(line.mark), text: line.text }));
  }

  private static invertMark(mark: DiffMark): DiffMark {
    if (mark === DiffMark.Added) return DiffMark.Removed;
    return mark === DiffMark.Removed ? DiffMark.Added : DiffMark.Changed;
  }
}
