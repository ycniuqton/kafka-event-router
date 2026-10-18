import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule } from '../core/HttpServiceModule';
import { HttpTeamsService } from './HttpTeamsService';

/** Service key of teams and grants. */
const TeamsKey = 'teams' as const satisfies keyof Services;

/** REST module for `services.teams` (auto-discovered by HttpServicesFactory). */
export class HttpTeamsModule extends HttpServiceModule<typeof TeamsKey> {
  readonly key = TeamsKey;

  create(context: HttpContext): Services[typeof TeamsKey] {
    return new HttpTeamsService(context);
  }
}

export default new HttpTeamsModule();
