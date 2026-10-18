import { actions } from '../../../store';
import { DocumentFormat } from '../../../domain';
import { RouteFileSaver } from './RouteFileSaver';
import type { RouteSelection } from './RouteSelection';

/** Bulk actions on the current selection; each one clears the selection afterwards. */
export class BulkRouteActions {
  private readonly selection: RouteSelection;
  private readonly clearSelection: () => void;

  constructor(selection: RouteSelection, clearSelection: () => void) {
    this.selection = selection;
    this.clearSelection = clearSelection;
  }

  pause(): void {
    void actions.routes.bulkPause(this.selection.toArray());
    this.clearSelection();
  }

  resume(): void {
    void actions.routes.bulkResume(this.selection.toArray());
    this.clearSelection();
  }

  exportYaml(): void {
    void actions.routes.exportRoutes(this.selection.toArray(), DocumentFormat.Yaml).then(RouteFileSaver.save, (e: Error) => actions.toast.show(e.message));
    this.clearSelection();
  }

  addTag(tag: string): void {
    void actions.routes.bulkTag(this.selection.toArray(), tag);
    this.clearSelection();
  }

  clear(): void {
    this.clearSelection();
  }
}
