import type { NavigateFunction } from 'react-router-dom';
import { AppPaths } from '../../../app/paths';
import { DocumentFormat, RouteDetailTab } from '../../../domain';
import { actions } from '../../../store';
import { RouteFileSaver } from './RouteFileSaver';

/** Row-level commands of a route (open, edit, metrics, pause/resume, duplicate, archive, unarchive, export). */
export class RouteCommands {
  private readonly navigate: NavigateFunction;
  private readonly confirmArchive: (id: string) => void;
  private readonly onListChange: () => void;

  /** `confirmArchive` opens the confirmation; `archiveNow` runs after the user confirms; `onListChange` reloads archived rows. */
  constructor(navigate: NavigateFunction, confirmArchive: (id: string) => void, onListChange: () => void = () => undefined) {
    this.navigate = navigate;
    this.confirmArchive = confirmArchive;
    this.onListChange = onListChange;
  }

  open(id: string): void {
    this.navigate(AppPaths.routeDetail(id));
  }

  edit(id: string): void {
    this.navigate(AppPaths.routeEdit(id));
  }

  viewMetrics(id: string): void {
    this.navigate(AppPaths.routeDetail(id, RouteDetailTab.Overview));
  }

  togglePause(id: string): void {
    void actions.routes.togglePause(id);
  }

  duplicate(id: string): void {
    void actions.routes.duplicate(id);
  }

  archive(id: string): void {
    this.confirmArchive(id);
  }

  archiveNow(id: string): void {
    void actions.routes.archive(id);
  }

  unarchive(id: string): void {
    void actions.routes.unarchive(id).then(() => this.onListChange());
  }

  exportYaml(id: string): void {
    void actions.routes.exportRoutes([id], DocumentFormat.Yaml).then(RouteFileSaver.save, (e: Error) => actions.toast.show(e.message));
  }
}
