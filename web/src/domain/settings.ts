/** Colour theme. */
export const Theme = { Dark: 'dark', Light: 'light' } as const;
export type Theme = (typeof Theme)[keyof typeof Theme];

/** Delivery phase a screen belongs to (shown as tags in the nav when enabled). */
export const Phase = { Mvp: 'MVP', P2: 'P2', P3: 'P3' } as const;
export type Phase = (typeof Phase)[keyof typeof Phase];

/** User preferences persisted in the browser. */
export type UserSettings = { theme: Theme; showPhases: boolean };

export const DefaultSettings: UserSettings = { theme: Theme.Dark, showPhases: true };

/** Workspace-wide settings (`/workspace-settings`); `version` is the If-Match token of changes. */
export type WorkspaceSettings = {
  aiAssistEnabled: boolean;
  defaultRequireProdApproval: boolean;
  /** Every route gets the "router-caused dead letters > 0" alert rule (enabled) unless turned off. */
  routerCausedAlertDefault?: boolean;
  version: number;
};

/** A change to workspace settings: only the given members are written. */
export type WorkspaceSettingsPatch = Partial<Omit<WorkspaceSettings, 'version'>>;
