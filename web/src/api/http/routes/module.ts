import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpRoutesService } from './HttpRoutesService';

/** REST module for `services.routes` over `/routes` (auto-discovered by HttpServicesFactory). */
export class HttpRoutesModule extends HttpServiceModule<typeof ServiceKeys.Routes> {
  readonly key = ServiceKeys.Routes;

  create(context: HttpContext): Services[typeof ServiceKeys.Routes] {
    return new HttpRoutesService(context);
  }
}

export default new HttpRoutesModule();
