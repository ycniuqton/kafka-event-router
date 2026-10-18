import type { Services } from '../../contracts';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpLiveTailService } from './HttpLiveTailService';

/** WS module for `services.liveTail` (auto-discovered by HttpServicesFactory). */
export class HttpLiveTailModule extends HttpServiceModule<typeof ServiceKeys.LiveTail> {
  readonly key = ServiceKeys.LiveTail;

  create(): Services[typeof ServiceKeys.LiveTail] {
    return new HttpLiveTailService();
  }
}

export default new HttpLiveTailModule();
