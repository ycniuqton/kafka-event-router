import { HeaderActionSlot } from '../model/HeaderActionSlot';
import { ReplayButton } from './ReplayButton';

/** Header actions owned by monitoring (Replay, P3), registered in routes' header action slot. */
const MonitoringHeaderAction = { id: 'monitoring.replay', order: 5 } as const;

/** Registers monitoring's header actions (imported once by the detail page). */
export class MonitoringHeaderActions {
  static readonly ready = MonitoringHeaderActions.registerAll();

  private static registerAll(): boolean {
    HeaderActionSlot.register({ id: MonitoringHeaderAction.id, order: MonitoringHeaderAction.order, Component: ReplayButton });
    return true;
  }
}
