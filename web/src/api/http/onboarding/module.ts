import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpOnboardingService } from './HttpOnboardingService';

/** REST module for `services.onboarding` (auto-discovered by HttpServicesFactory). */
export class HttpOnboardingModule extends HttpServiceModule<typeof ServiceKeys.Onboarding> {
  readonly key = ServiceKeys.Onboarding;

  create(context: HttpContext): Services[typeof ServiceKeys.Onboarding] {
    return new HttpOnboardingService(context);
  }
}

export default new HttpOnboardingModule();
