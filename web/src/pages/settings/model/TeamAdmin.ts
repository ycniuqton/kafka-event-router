import { services } from '../../../api';
import { TeamRules, type Connection, type ConnectionGrant, type Team, type TeamDraft } from '../../../domain';

/** Text of the team administration toasts. */
export const TeamAdminText = {
  Created: (name: string) => `Team ${name} created`,
  Saved: (name: string) => `Team ${name} saved`,
  Deleted: (name: string) => `Team ${name} deleted`,
  Granted: (conn: string, team: string) => `${team} can now use ${conn}`,
  Revoked: (conn: string, team: string) => `${team} can no longer use ${conn}`,
} as const;

/** Policy labels of a team row. */
export const ApprovalPolicyLabel = { Required: 'Prod deploys need approval', Direct: 'Approvers deploy to prod directly' } as const;

/** Team editor and grants editor actions: each resolves with the toast text or rejects with the server's message. */
export class TeamAdmin {
  static blankDraft(): TeamDraft {
    return { name: '', requireProdApproval: true };
  }

  static draftOf(team: Team): TeamDraft {
    return { name: team.name, requireProdApproval: team.requireProdApproval };
  }

  static canSave(draft: TeamDraft): boolean {
    return TeamRules.name(draft.name) !== null;
  }

  static async save(team: Team | null, draft: TeamDraft): Promise<string> {
    if (!team) return TeamAdminText.Created((await services.teams.create(draft)).name);
    return TeamAdminText.Saved((await services.teams.update(team, draft)).name);
  }

  static async remove(team: Team): Promise<string> {
    await services.teams.remove(team);
    return TeamAdminText.Deleted(team.name);
  }

  static async grant(team: Team, connection: Connection): Promise<string> {
    const g = await services.teams.addGrant(team.id, connection.id);
    return TeamAdminText.Granted(g.connectionName, team.name);
  }

  static async revoke(team: Team, grant: ConnectionGrant): Promise<string> {
    await services.teams.removeGrant(team.id, grant.connectionId);
    return TeamAdminText.Revoked(grant.connectionName, team.name);
  }

  /** Connections another team owns that this team does not hold a grant on yet. */
  static grantable(team: Team, connections: readonly Connection[], grants: readonly ConnectionGrant[]): Connection[] {
    const held = new Set(grants.map((g) => g.connectionId));
    return connections.filter((c) => c.owner !== team.name && c.owner !== team.id && !held.has(c.id));
  }
}
