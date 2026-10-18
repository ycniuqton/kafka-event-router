import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpApiTokensService } from './HttpApiTokensService';

/** REST module for `services.apiTokens` (auto-discovered by HttpServicesFactory). */
export class HttpApiTokensModule extends HttpServiceModule<typeof ServiceKeys.ApiTokens> {
  readonly key = ServiceKeys.ApiTokens;

  create(context: HttpContext): Services[typeof ServiceKeys.ApiTokens] {
    return new HttpApiTokensService(context);
  }
}

export default new HttpApiTokensModule();
