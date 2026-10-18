import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule } from '../core/HttpServiceModule';
import { HttpAccessService } from './HttpAccessService';

/** Service key of member administration and effective access. */
const AccessKey = 'access' as const satisfies keyof Services;

/** REST module for `services.access` (auto-discovered by HttpServicesFactory). */
export class HttpAccessModule extends HttpServiceModule<typeof AccessKey> {
  readonly key = AccessKey;

  create(context: HttpContext): Services[typeof AccessKey] {
    return new HttpAccessService(context);
  }
}

export default new HttpAccessModule();
