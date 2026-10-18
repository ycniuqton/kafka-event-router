import type { EnvTag } from './connection';

/** A team: owns routes, connections and transforms; its policy decides whether prod deploys need approval. */
export type Team = {
  id: string;
  name: string;
  requireProdApproval: boolean;
  memberCount: number;
  createdAt: string;
  rowVersion: number;
};

/** Fields of the team editor (`POST /teams`, `PATCH /teams/{id}`). */
export type TeamDraft = { name: string; requireProdApproval: boolean };

/** A grant: the team may use a connection owned by another team (`/teams/{id}/grants`). */
export type ConnectionGrant = {
  teamId: string;
  connectionId: string;
  connectionName: string;
  ownerTeamId: string;
  envTag: EnvTag;
  grantedBy: string | null;
  createdAt: string;
};

/** Pre-invitation of a user by email with one team role (`POST /users/invitations`). */
export type InviteDraft = { email: string; displayName: string; teamId: string; role: string };

/** Team naming and lookup rules shared by the editor and the mocks. */
export class TeamRules {
  static readonly maxName = 100;

  /** Trimmed name, or null when empty / too long (server: 422 /name). */
  static name(raw: string): string | null {
    const name = raw.trim();
    return name && name.length <= TeamRules.maxName ? name : null;
  }

  /** The team with this id or (for API rows that carry a name) this name. */
  static find(teams: readonly Team[], idOrName: string | null | undefined): Team | null {
    if (!idOrName) return null;
    return teams.find((t) => t.id === idOrName || t.name === idOrName) ?? null;
  }
}
