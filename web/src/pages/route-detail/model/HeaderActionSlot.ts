import type { ComponentType } from 'react';
import type { Route } from '../../../domain';

/** An action another feature adds to the route detail header (e.g. Export, P2). */
export type HeaderAction = { id: string; order: number; Component: ComponentType<{ route: Route }> };

/** Action slot of the route detail header: owners register buttons, the header renders them before "Edit route". */
export class HeaderActionSlot {
  private static readonly actions = new Map<string, HeaderAction>();

  static register(action: HeaderAction): void {
    HeaderActionSlot.actions.set(action.id, action);
  }

  static all(): HeaderAction[] {
    return [...HeaderActionSlot.actions.values()].sort((a, b) => a.order - b.order);
  }
}
