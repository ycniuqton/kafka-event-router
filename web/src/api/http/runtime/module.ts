import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpRuntimeService } from './HttpRuntimeService';

/** REST module for `services.runtime` (auto-discovered by HttpServicesFactory). */
export class HttpRuntimeModule extends HttpServiceModule<typeof ServiceKeys.Runtime> {
  readonly key = ServiceKeys.Runtime;

  create(context: HttpContext): Services[typeof ServiceKeys.Runtime] {
    return new HttpRuntimeService(context);
  }
}

export default new HttpRuntimeModule();
