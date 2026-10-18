import type { Services } from '../../contracts';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpPushEventsService } from './HttpPushEventsService';

/** REST/WS module for `services.push` (auto-discovered by HttpServicesFactory). */
export class HttpPushModule extends HttpServiceModule<typeof ServiceKeys.Push> {
  readonly key = ServiceKeys.Push;

  create(): Services[typeof ServiceKeys.Push] {
    return new HttpPushEventsService();
  }
}

export default new HttpPushModule();
