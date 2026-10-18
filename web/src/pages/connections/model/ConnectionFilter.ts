import { ConnectionDisplay, ConnectionStatus, EnvTag, type Connection } from '../../../domain';

/** "All" option of the env / status filters. */
export const FilterAll = 'all' as const;
export type FilterAll = typeof FilterAll;

/** Filter state of the connections list (CN-01: env, status, text). */
export type ConnectionFilterState = { q: string; env: EnvTag | FilterAll; status: ConnectionStatus | FilterAll };

/** Applies and counts the list filters (the list is loaded whole, so filtering is local and instant). */
export class ConnectionFilter {
  static readonly initial: ConnectionFilterState = { q: '', env: FilterAll, status: FilterAll };
  static readonly envOptions = [FilterAll, EnvTag.Dev, EnvTag.Staging, EnvTag.Prod] as const;
  static readonly statusOptions = [
    FilterAll,
    ConnectionStatus.Healthy,
    ConnectionStatus.Degraded,
    ConnectionStatus.Unreachable,
    ConnectionStatus.AuthFailed,
    ConnectionStatus.Untested,
  ] as const;

  static apply(connections: Connection[], f: ConnectionFilterState): Connection[] {
    return connections.filter((c) => ConnectionFilter.matches(c, f));
  }

  static isActive(f: ConnectionFilterState): boolean {
    return !!f.q.trim() || f.env !== FilterAll || f.status !== FilterAll;
  }

  /** How many connections each status would show with the other filters kept. */
  static statusCount(connections: Connection[], f: ConnectionFilterState, status: ConnectionStatus | FilterAll): number {
    return ConnectionFilter.apply(connections, { ...f, status }).length;
  }

  private static matches(c: Connection, f: ConnectionFilterState): boolean {
    const q = f.q.trim().toLowerCase();
    const text = [ConnectionDisplay.name(c), c.description ?? '', ...c.bootstrapServers].join(' ').toLowerCase();
    if (q && !text.includes(q)) return false;
    if (f.env !== FilterAll && c.env !== f.env) return false;
    return f.status === FilterAll || c.status === f.status;
  }
}
