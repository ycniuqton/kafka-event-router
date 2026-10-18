import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpAlertsService } from './HttpAlertsService';

/** REST module for `services.alerts` (auto-discovered by HttpServicesFactory). */
export class HttpAlertsModule extends HttpServiceModule<typeof ServiceKeys.Alerts> {
  readonly key = ServiceKeys.Alerts;

  create(context: HttpContext): Services[typeof ServiceKeys.Alerts] {
    return new HttpAlertsService(context);
  }
}

export default new HttpAlertsModule();
