import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpActivityService } from './HttpActivityService';

/** REST module for `services.activity` (auto-discovered by HttpServicesFactory). */
export class HttpActivityModule extends HttpServiceModule<typeof ServiceKeys.Activity> {
  readonly key = ServiceKeys.Activity;

  create(context: HttpContext): Services[typeof ServiceKeys.Activity] {
    return new HttpActivityService(context);
  }
}

export default new HttpActivityModule();
