import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpDeadLettersService } from './HttpDeadLettersService';

/** REST module for `services.deadLetters` over `/routes/{id}/dlq` and `/dlq` (auto-discovered). */
export class HttpDeadLettersModule extends HttpServiceModule<typeof ServiceKeys.DeadLetters> {
  readonly key = ServiceKeys.DeadLetters;

  create(context: HttpContext): Services[typeof ServiceKeys.DeadLetters] {
    return new HttpDeadLettersService(context);
  }
}

export default new HttpDeadLettersModule();
