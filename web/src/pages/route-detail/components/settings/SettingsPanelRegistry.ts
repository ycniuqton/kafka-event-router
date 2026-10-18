import type { ComponentType } from 'react';
import type { Route } from '../../../../domain';

/** A panel another feature adds below the route settings (e.g. monitoring's alert rules, P2). */
export type SettingsPanel = { id: string; order: number; Component: ComponentType<{ route: Route }> };

/** Extension slot of the Settings tab: owners register panels, the tab renders them in order. */
export class SettingsPanelRegistry {
  private static readonly panels = new Map<string, SettingsPanel>();

  static register(panel: SettingsPanel): void {
    SettingsPanelRegistry.panels.set(panel.id, panel);
  }

  static all(): SettingsPanel[] {
    return [...SettingsPanelRegistry.panels.values()].sort((a, b) => a.order - b.order);
  }
}
