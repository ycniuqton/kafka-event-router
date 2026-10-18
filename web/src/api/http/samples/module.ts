import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpSamplesService } from './HttpSamplesService';

/** REST module for `services.samples` over `/preview/*` (auto-discovered by HttpServicesFactory). */
export class HttpSamplesModule extends HttpServiceModule<typeof ServiceKeys.Samples> {
  readonly key = ServiceKeys.Samples;

  create(context: HttpContext): Services[typeof ServiceKeys.Samples] {
    return new HttpSamplesService(context);
  }
}

export default new HttpSamplesModule();
