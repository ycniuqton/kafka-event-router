import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpConnectionsService } from './HttpConnectionsService';

/** REST module for `services.connections` over `/connections` (auto-discovered by HttpServicesFactory). */
export class HttpConnectionsModule extends HttpServiceModule<typeof ServiceKeys.Connections> {
  readonly key = ServiceKeys.Connections;

  create(context: HttpContext): Services[typeof ServiceKeys.Connections] {
    return new HttpConnectionsService(context);
  }
}

export default new HttpConnectionsModule();
