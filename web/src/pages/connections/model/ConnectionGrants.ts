/** A team granted use of a connection by access-control (`team_connection_grants`). */
export type GrantedTeam = { teamId: string; teamName: string };

/** PORT to access-control's team grants (AC-104 / AC-122): teams other than the owner that may use a connection. */
export interface ConnectionGrantsSource {
  /** Granted teams, or null while the grants API is not available. */
  grantedTeams(connectionId: string): Promise<GrantedTeam[] | null>;
}

/** Fallback until access-control registers its grants source: grants are unknown. */
export class UnavailableGrantsSource implements ConnectionGrantsSource {
  async grantedTeams(): Promise<GrantedTeam[] | null> {
    return null;
  }
}

/** Holds the configured grants source; access-control plugs its implementation in with `use`. */
export class ConnectionGrants {
  private static source: ConnectionGrantsSource = new UnavailableGrantsSource();

  static use(source: ConnectionGrantsSource): void {
    ConnectionGrants.source = source;
  }

  static current(): ConnectionGrantsSource {
    return ConnectionGrants.source;
  }
}

/** Texts of the read-only granted-teams list in the Basics step. */
export const GrantsText = {
  label: 'Also used by teams',
  none: 'Only the owner team can use this connection in routes.',
  unavailable: 'Other teams can use this connection once a workspace admin grants it to them.',
  manage: 'Manage team grants',
  loading: 'Loading team grants…',
} as const;
