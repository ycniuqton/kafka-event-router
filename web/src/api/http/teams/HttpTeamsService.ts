import type { ConnectionGrant, Team, TeamDraft } from '../../../domain';
import { TeamsService } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { EntityTag } from '../members/MemberDtos';

/** REST paths of teams and grants (relative to `/api/v1`). */
export class TeamsEndpoint {
  static readonly teams = '/teams';

  static team(id: string): string {
    return `${TeamsEndpoint.teams}/${encodeURIComponent(id)}`;
  }

  static grants(teamId: string): string {
    return `${TeamsEndpoint.team(teamId)}/grants`;
  }

  static grant(teamId: string, connectionId: string): string {
    return `${TeamsEndpoint.grants(teamId)}/${encodeURIComponent(connectionId)}`;
  }
}

/** `{items}` list body. */
type Items<T> = { items: T[] };

/** Teams and connection grants over `/teams` (If-Match on changes). */
export class HttpTeamsService extends TeamsService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async list(): Promise<Team[]> {
    return (await this.context.api.get<Items<Team>>(TeamsEndpoint.teams)).items;
  }

  create(draft: TeamDraft): Promise<Team> {
    return this.context.api.post<Team>(TeamsEndpoint.teams, { name: draft.name.trim(), requireProdApproval: draft.requireProdApproval });
  }

  update(team: Team, draft: TeamDraft): Promise<Team> {
    return this.context.api.patch<Team>(TeamsEndpoint.team(team.id), { name: draft.name.trim(), requireProdApproval: draft.requireProdApproval }, {
      ifMatch: EntityTag.of(team.rowVersion),
    });
  }

  async remove(team: Team): Promise<void> {
    await this.context.api.delete(TeamsEndpoint.team(team.id), { ifMatch: EntityTag.of(team.rowVersion) });
  }

  async grants(teamId: string): Promise<ConnectionGrant[]> {
    return (await this.context.api.get<Items<ConnectionGrant>>(TeamsEndpoint.grants(teamId))).items;
  }

  addGrant(teamId: string, connectionId: string): Promise<ConnectionGrant> {
    return this.context.api.put<ConnectionGrant>(TeamsEndpoint.grant(teamId, connectionId));
  }

  async removeGrant(teamId: string, connectionId: string): Promise<void> {
    await this.context.api.delete(TeamsEndpoint.grant(teamId, connectionId));
  }
}
