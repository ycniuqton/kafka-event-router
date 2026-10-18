import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpPreviewService } from './HttpPreviewService';

/** REST module for `services.preview` over `/preview/*` (auto-discovered by HttpServicesFactory). */
export class HttpPreviewModule extends HttpServiceModule<typeof ServiceKeys.Preview> {
  readonly key = ServiceKeys.Preview;

  create(context: HttpContext): Services[typeof ServiceKeys.Preview] {
    return new HttpPreviewService(context);
  }
}

export default new HttpPreviewModule();
