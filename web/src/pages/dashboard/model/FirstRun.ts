import { AppPaths } from '../../../app/paths';
import { BuilderStep, ConnectionDisplay, RouteStatus, type Connection, type Route } from '../../../domain';
import { actions } from '../../../store';

export type FirstRunState = { connections: Connection[]; routes: Route[] };
export type FirstRunNavigate = (path: string) => void;

/** One step of the new-workspace checklist (spec §12). */
export abstract class FirstRunStep {
  abstract readonly id: string;
  abstract readonly title: string;
  abstract readonly description: string;
  abstract readonly actionLabel: string;
  abstract isDone(state: FirstRunState): boolean;
  abstract perform(state: FirstRunState, navigate: FirstRunNavigate): void;
}

class AddConnectionStep extends FirstRunStep {
  readonly id = 'connection';
  readonly title = 'Add your first connection';
  readonly description = 'Enter a Kafka cluster once and reuse it in every route.';
  readonly actionLabel = 'Add connection';

  isDone(state: FirstRunState): boolean {
    return state.connections.some((c) => !ConnectionDisplay.isDemo(c));
  }

  perform(_state: FirstRunState, navigate: FirstRunNavigate): void {
    navigate(AppPaths.connections());
  }
}

class CreateRouteStep extends FirstRunStep {
  readonly id = 'route';
  readonly title = 'Create your first route';
  readonly description = 'Pick a source topic, filter and reshape events, choose a destination.';
  readonly actionLabel = '+ New route';

  isDone(state: FirstRunState): boolean {
    return state.routes.length > 0;
  }

  perform(): void {
    actions.openNewRouteChooser();
  }
}

class DeployStep extends FirstRunStep {
  readonly id = 'deploy';
  readonly title = 'Deploy it';
  readonly description = 'Review the summary and start forwarding events.';
  readonly actionLabel = 'Review & deploy';

  isDone(state: FirstRunState): boolean {
    return state.routes.some((r) => r.status !== RouteStatus.Draft);
  }

  perform(state: FirstRunState, navigate: FirstRunNavigate): void {
    const draft = state.routes.find((r) => r.status === RouteStatus.Draft);
    navigate(draft ? AppPaths.routeEdit(draft.id, BuilderStep.Review) : AppPaths.routes());
  }
}

/** Owns the ordered checklist and decides whether the workspace still needs it. */
export class FirstRunChecklistManager {
  static readonly steps: FirstRunStep[] = [new AddConnectionStep(), new CreateRouteStep(), new DeployStep()];

  static isComplete(state: FirstRunState): boolean {
    return FirstRunChecklistManager.steps.every((s) => s.isDone(state));
  }

  /** Index of the first unfinished step (the one to highlight). */
  static currentIndex(state: FirstRunState): number {
    return FirstRunChecklistManager.steps.findIndex((s) => !s.isDone(state));
  }
}
