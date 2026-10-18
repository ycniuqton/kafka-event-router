import { HeaderActionSlot } from '../../model/HeaderActionSlot';
import { ExportAction } from './ExportAction';
import { UnarchiveAction } from './UnarchiveAction';

/** Header action ids and order of the routes feature (other features register their own). */
const HeaderActionOrder = { Unarchive: 10, Export: 20 } as const;
const HeaderActionId = { Unarchive: 'routes.unarchive', Export: 'routes.export' } as const;

/** Registers the routes feature's header actions (imported once by the detail page). */
export class RegisteredHeaderActions {
  static readonly ready = RegisteredHeaderActions.registerAll();

  private static registerAll(): boolean {
    HeaderActionSlot.register({ id: HeaderActionId.Unarchive, order: HeaderActionOrder.Unarchive, Component: UnarchiveAction });
    HeaderActionSlot.register({ id: HeaderActionId.Export, order: HeaderActionOrder.Export, Component: ExportAction });
    return true;
  }
}
