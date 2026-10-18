import type { Services } from '../../contracts';
import { SystemClock } from '../../../domain';
import { HttpServiceModule, ServiceKeys } from './HttpServiceModule';

/** The live API uses wall-clock time. */
export class HttpClockModule extends HttpServiceModule<typeof ServiceKeys.Clock> {
  readonly key = ServiceKeys.Clock;

  create(): Services[typeof ServiceKeys.Clock] {
    return new SystemClock();
  }
}
