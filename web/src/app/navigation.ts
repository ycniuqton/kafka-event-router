import { Permission, Phase } from '../domain';
import type { AppState } from '../store';
import { AppPaths } from './paths';

/** Left-nav entries (design `NAV`). */
export const NavKey = {
  Dashboard: 'dashboard',
  Routes: 'routes',
  Connections: 'connections',
  Library: 'library',
  DeadLetters: 'deadletters',
  Audit: 'audit',
  Settings: 'settings',
} as const;
export type NavKey = (typeof NavKey)[keyof typeof NavKey];

export type NavItem = {
  key: NavKey;
  label: string;
  path: string;
  phase: Phase;
  /** Store-derived count shown next to the label. */
  count?: (state: AppState) => number;
  /** Shown only to callers holding this permission (access-control). */
  permission?: Permission;
};

export const NavItems: NavItem[] = [
  { key: NavKey.Dashboard, label: 'Dashboard', path: AppPaths.dashboard(), phase: Phase.Mvp },
  { key: NavKey.Routes, label: 'Routes', path: AppPaths.routes(), phase: Phase.Mvp, count: (s) => s.routes.length },
  { key: NavKey.Connections, label: 'Connections', path: AppPaths.connections(), phase: Phase.Mvp, count: (s) => s.connections.length },
  { key: NavKey.Library, label: 'Transform library', path: AppPaths.library(), phase: Phase.P3 },
  { key: NavKey.DeadLetters, label: 'Dead letters', path: AppPaths.deadLetters(), phase: Phase.Mvp, count: (s) => s.deadLetterSummary.unresolved },
  { key: NavKey.Audit, label: 'Audit log', path: AppPaths.audit(), phase: Phase.P3, permission: Permission.AuditRead },
  { key: NavKey.Settings, label: 'Settings', path: AppPaths.settings(), phase: Phase.Mvp },
];

/** Per-route metadata read by the shell via `useMatches()`. */
export type RouteHandle = {
  nav?: NavKey;
  /** Hide the left nav and let the page fill the main area (route builder). */
  fullscreen?: boolean;
};
