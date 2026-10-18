import { services } from '../../../api';
import { DemoDataText, type OnboardingState } from '../../../domain';
import { actions } from '../../../store';

/** "Try with demo data": creates / removes the team's in-memory demo connection and refreshes connections. */
export class DemoDataActions {
  static load(): Promise<OnboardingState> {
    return services.onboarding.get();
  }

  static async create(): Promise<OnboardingState> {
    try {
      await services.onboarding.createDemo();
      actions.toast.show(DemoDataText.created);
    } catch (e) {
      actions.toast.show((e as Error).message || DemoDataText.exists);
    }
    return DemoDataActions.reload();
  }

  static async remove(): Promise<OnboardingState> {
    try {
      await services.onboarding.removeDemo();
      actions.toast.show(DemoDataText.removed);
    } catch (e) {
      actions.toast.show((e as Error).message);
    }
    return DemoDataActions.reload();
  }

  private static async reload(): Promise<OnboardingState> {
    await actions.connections.refresh().catch(() => undefined);
    return services.onboarding.get();
  }
}
