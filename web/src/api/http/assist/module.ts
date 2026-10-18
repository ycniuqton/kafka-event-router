import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpAssistService } from './HttpAssistService';

/** REST module for `services.assist` over `/assist/*` (auto-discovered by HttpServicesFactory). */
export class HttpAssistModule extends HttpServiceModule<typeof ServiceKeys.Assist> {
  readonly key = ServiceKeys.Assist;

  create(context: HttpContext): Services[typeof ServiceKeys.Assist] {
    return new HttpAssistService(context);
  }
}

export default new HttpAssistModule();
