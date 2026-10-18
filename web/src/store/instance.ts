import { services } from '../api';
import { AppActions } from './AppActions';
import { AppStore } from './AppStore';
import { SettingsStore } from './SettingsStore';

/** Application-wide singletons. Prefer the hooks in `store/hooks.ts` inside components. */
export const appStore = new AppStore();
export const actions = new AppActions(appStore, services);
export const settingsStore = new SettingsStore();
