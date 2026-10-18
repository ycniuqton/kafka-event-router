import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpLibraryService } from './HttpLibraryService';

/** REST module for `services.library` over `/transforms` and `/lookup-tables` (auto-discovered by HttpServicesFactory). */
export class HttpLibraryModule extends HttpServiceModule<typeof ServiceKeys.Library> {
  readonly key = ServiceKeys.Library;

  create(context: HttpContext): Services[typeof ServiceKeys.Library] {
    return new HttpLibraryService(context);
  }
}

export default new HttpLibraryModule();
