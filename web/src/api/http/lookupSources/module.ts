import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpLookupSourcesService } from './HttpLookupSourcesService';

/** REST module for `services.lookupSources` over `/lookup-sources` (auto-discovered by HttpServicesFactory). */
export class HttpLookupSourcesModule extends HttpServiceModule<typeof ServiceKeys.LookupSources> {
  readonly key = ServiceKeys.LookupSources;

  create(context: HttpContext): Services[typeof ServiceKeys.LookupSources] {
    return new HttpLookupSourcesService(context);
  }
}

export default new HttpLookupSourcesModule();
