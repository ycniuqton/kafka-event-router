import { ConnectionDisplay, ConnectionStatusMeta, type Connection, type ConnectionRouteRef, type Route, type Tone } from '../../../domain';
import { Format } from '../../../components/ui';

/** One row of the connections table, derived from a connection and the routes using it. */
export type ConnectionRowView = {
  connection: Connection;
  firstServer: string;
  moreServers: string;
  statusLabel: string;
  statusTone: Tone;
  statusTitle: string | undefined;
  checked: string;
  usedByCount: number;
  usedBy: ConnectionRouteRef[];
  deleteTitle: string;
  editable: boolean;
};

/** Builds connection row view-models (design `conns` mapping); the server's used-by count wins over local routes. */
export class ConnectionRowModel {
  static readonly deleteTitle = 'Delete connection';
  static readonly inUsePrefix = 'In use by: ';
  static readonly listSeparator = ', ';
  static readonly morePrefix = '+';
  static readonly lineBreak = '\n';
  static readonly routesSuffix = ' routes';

  static usedBy(connectionId: string, routes: Route[]): ConnectionRouteRef[] {
    return routes
      .filter((r) => r.source.connectionId === connectionId || r.destination.connectionId === connectionId)
      .map((r) => ({ id: r.id, name: r.name }));
  }

  static build(connection: Connection, routes: Route[], now: Date): ConnectionRowView {
    const usedBy = connection.usedBy?.routes ?? ConnectionRowModel.usedBy(connection.id, routes);
    const usedByCount = connection.usedBy?.count ?? usedBy.length;
    const extra = connection.bootstrapServers.length - 1;
    const meta = ConnectionStatusMeta[connection.status];
    const failure = connection.lastFailure;
    return {
      connection,
      firstServer: connection.bootstrapServers[0] ?? Format.placeholder,
      moreServers: extra > 0 ? `${ConnectionRowModel.morePrefix}${extra}` : '',
      statusLabel: meta.label,
      statusTone: meta.tone,
      statusTitle: failure ? [failure.message, failure.hint].filter(Boolean).join(ConnectionRowModel.lineBreak) : undefined,
      checked: Format.relativeTime(connection.checkedAt, now),
      usedByCount,
      usedBy,
      deleteTitle: usedByCount ? ConnectionRowModel.inUseTitle(usedBy, usedByCount) : ConnectionRowModel.deleteTitle,
      editable: ConnectionDisplay.editable(connection),
    };
  }

  private static inUseTitle(usedBy: ConnectionRouteRef[], count: number): string {
    const names = usedBy.map((r) => r.name).join(ConnectionRowModel.listSeparator);
    return ConnectionRowModel.inUsePrefix + (usedBy.length >= count ? names : `${count}${ConnectionRowModel.routesSuffix}`);
  }
}
