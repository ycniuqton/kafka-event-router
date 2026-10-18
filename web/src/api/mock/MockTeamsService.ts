import { TeamRules, type ConnectionGrant, type Team, type TeamDraft } from '../../domain';
import { CURRENT_USER_ID, GRANTS, TEAMS } from '../../mocks';
import { TeamsService } from '../contracts';
import type { MockBackend } from './MockBackend';

/** Messages of the mock team rules (same wording as the API problems). */
export const MockTeamsText = {
  InvalidName: 'Enter a team name of 1 to 100 characters.',
  NameTaken: 'Another team already uses this name. Pick a different name.',
  NotEmpty: 'This team still owns routes or connections. Move or delete them first.',
  Stale: 'Someone else changed this. Reload to see their version, then retry.',
  GrantToOwner: 'This team already owns the connection, so it needs no grant.',
  NotFound: 'The requested resource does not exist. It may have been deleted.',
} as const;

/** In-memory teams and grants enforcing the API rules (unique names, versions, owner grants, non-empty delete). */
export class MockTeamsService extends TeamsService {
  private readonly backend: MockBackend;
  private teams: Team[] = structuredClone(TEAMS);
  private grantRows: ConnectionGrant[] = structuredClone(GRANTS);

  constructor(backend: MockBackend) {
    super();
    this.backend = backend;
  }

  list(): Promise<Team[]> {
    return this.backend.reply([...this.teams].sort((a, b) => a.name.localeCompare(b.name)));
  }

  create(draft: TeamDraft): Promise<Team> {
    const name = TeamRules.name(draft.name);
    if (!name) return this.backend.fail(MockTeamsText.InvalidName);
    if (this.taken(name, null)) return this.backend.fail(MockTeamsText.NameTaken);
    const team: Team = { id: `team-${name.toLowerCase().replace(/\W+/g, '-')}`, name, requireProdApproval: draft.requireProdApproval, memberCount: 0,
      createdAt: this.backend.nowIso(), rowVersion: 1 };
    this.teams.push(team);
    return this.backend.reply(team);
  }

  update(team: Team, draft: TeamDraft): Promise<Team> {
    const row = this.teams.find((t) => t.id === team.id);
    const name = TeamRules.name(draft.name);
    if (!row) return this.backend.fail(MockTeamsText.NotFound);
    if (row.rowVersion !== team.rowVersion) return this.backend.fail(MockTeamsText.Stale);
    if (!name) return this.backend.fail(MockTeamsText.InvalidName);
    if (this.taken(name, row.id)) return this.backend.fail(MockTeamsText.NameTaken);
    Object.assign(row, { name, requireProdApproval: draft.requireProdApproval, rowVersion: row.rowVersion + 1 });
    return this.backend.reply(row);
  }

  remove(team: Team): Promise<void> {
    const owns = this.backend.db.routes.some((r) => r.owner === team.name) || this.backend.db.connections.some((c) => c.owner === team.name);
    if (owns) return this.backend.fail(MockTeamsText.NotEmpty);
    this.teams = this.teams.filter((t) => t.id !== team.id);
    this.grantRows = this.grantRows.filter((g) => g.teamId !== team.id);
    return this.backend.reply(undefined);
  }

  grants(teamId: string): Promise<ConnectionGrant[]> {
    return this.backend.reply(this.grantRows.filter((g) => g.teamId === teamId));
  }

  addGrant(teamId: string, connectionId: string): Promise<ConnectionGrant> {
    const conn = this.backend.db.connections.find((c) => c.id === connectionId);
    const team = this.teams.find((t) => t.id === teamId);
    if (!conn || !team) return this.backend.fail(MockTeamsText.NotFound);
    if (conn.owner === team.name) return this.backend.fail(MockTeamsText.GrantToOwner);
    const existing = this.grantRows.find((g) => g.teamId === teamId && g.connectionId === connectionId);
    if (existing) return this.backend.reply(existing);
    const owner = TeamRules.find(this.teams, conn.owner);
    const grant: ConnectionGrant = { teamId, connectionId, connectionName: conn.name ?? conn.id, ownerTeamId: owner?.id ?? '', envTag: conn.env,
      grantedBy: CURRENT_USER_ID, createdAt: this.backend.nowIso() };
    this.grantRows.push(grant);
    return this.backend.reply(grant);
  }

  removeGrant(teamId: string, connectionId: string): Promise<void> {
    const before = this.grantRows.length;
    this.grantRows = this.grantRows.filter((g) => !(g.teamId === teamId && g.connectionId === connectionId));
    return before === this.grantRows.length ? this.backend.fail(MockTeamsText.NotFound) : this.backend.reply(undefined);
  }

  private taken(name: string, except: string | null): boolean {
    return this.teams.some((t) => t.id !== except && t.name.toLowerCase() === name.toLowerCase());
  }
}
