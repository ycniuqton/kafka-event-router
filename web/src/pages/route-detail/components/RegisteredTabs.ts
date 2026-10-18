import { RouteDetailTab } from '../../../domain';
import { DetailTabs } from '../model/DetailTabs';
import { DeadLettersTab } from './DeadLettersTab';
import { HistoryTab } from './HistoryTab';
import { LiveTailTab } from './LiveTailTab';
import { OverviewTab } from './OverviewTab';
import { SettingsTab } from './settings/SettingsTab';
import { SettingsPanelRegistry } from './settings/SettingsPanelRegistry';
import { AlertRulesSettingsPanel } from '../settings/AlertRuleFormModel';
import { AlertRulesPanel } from '../settings/AlertRulesPanel';

/** Tab order of the route detail page (spec §10–11 + FR-44 Settings). */
const TabOrder = { Overview: 10, Tail: 20, Dead: 30, History: 40, Settings: 50 } as const;

/** Registers the built-in tabs of the route detail page (imported once by the page). */
export class RegisteredTabs {
  static readonly ready = RegisteredTabs.registerAll();

  private static registerAll(): boolean {
    DetailTabs.register({ id: RouteDetailTab.Overview, order: TabOrder.Overview, label: () => 'Overview', Component: OverviewTab });
    DetailTabs.register({ id: RouteDetailTab.Tail, order: TabOrder.Tail, label: () => 'Live tail', Component: LiveTailTab });
    DetailTabs.register({ id: RouteDetailTab.Dead, order: TabOrder.Dead, label: ({ deadLetters }) => `Dead letters · ${deadLetters}`, Component: DeadLettersTab });
    DetailTabs.register({ id: RouteDetailTab.History, order: TabOrder.History, label: () => 'History', Component: HistoryTab });
    DetailTabs.register({ id: RouteDetailTab.Settings, order: TabOrder.Settings, label: () => 'Settings', Component: SettingsTab });
    SettingsPanelRegistry.register({ id: AlertRulesSettingsPanel.id, order: AlertRulesSettingsPanel.order, Component: AlertRulesPanel });
    return true;
  }
}
