import { DeploymentStatus, Tone, VersionOrigin, type RouteVersion } from '../../../domain';

type StatusLook = { label: string; tone: Tone };

/** History labels of a version: why it exists (rollback, auto-rollback, import) and its deployment state. */
export class VersionPresentation {
  static readonly runningLabel = 'running';

  private static readonly statuses: Partial<Record<DeploymentStatus, StatusLook>> = {
    [DeploymentStatus.Failed]: { label: 'failed', tone: Tone.Err },
    [DeploymentStatus.RollingOut]: { label: 'rolling out', tone: Tone.Info },
    [DeploymentStatus.PendingApproval]: { label: 'pending approval', tone: Tone.Warn },
    [DeploymentStatus.Rejected]: { label: 'rejected', tone: Tone.Muted },
    [DeploymentStatus.Cancelled]: { label: 'cancelled', tone: Tone.Muted },
  };

  /** "Rollback to v7", "Auto-rollback to v7", "Imported", or null for a plain deploy. */
  static origin(version: RouteVersion): string | null {
    const base = version.basedOn ? ` to v${version.basedOn}` : '';
    if (version.origin === VersionOrigin.Rollback) return `Rollback${base}`;
    if (version.origin === VersionOrigin.AutoRollback) return `Auto-rollback${base}`;
    if (version.origin === VersionOrigin.Import) return 'Imported';
    return null;
  }

  /** Deployment state worth showing (failed, rolling out, pending…); null for active/superseded. */
  static status(version: RouteVersion): StatusLook | null {
    return version.deploymentStatus ? (VersionPresentation.statuses[version.deploymentStatus] ?? null) : null;
  }

  /** The running version: the server's `active` flag, else the newest (mock data). */
  static current(versions: RouteVersion[]): RouteVersion | undefined {
    return versions.find((v) => v.active) ?? (versions.some((v) => v.active !== undefined) ? undefined : versions[0]);
  }
}
