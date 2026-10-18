import type { DemoSeed, OnboardingState } from '../../../domain';
import { OnboardingService } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';

/** Paths of the first-run endpoints (relative to `/api/v1`). */
export const OnboardingEndpoint = { state: '/onboarding', demo: '/onboarding/demo' } as const;

/** `services.onboarding` over `/api/v1/onboarding` and `/onboarding/demo`. */
export class HttpOnboardingService extends OnboardingService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async get(): Promise<OnboardingState> {
    const dto = await this.context.api.get<OnboardingState>(OnboardingEndpoint.state);
    return { allDone: dto.allDone, demoAvailable: dto.demoAvailable, demo: dto.demo ?? null };
  }

  createDemo(): Promise<DemoSeed> {
    return this.context.api.post<DemoSeed>(OnboardingEndpoint.demo);
  }

  removeDemo(): Promise<void> {
    return this.context.api.delete(OnboardingEndpoint.demo);
  }
}
