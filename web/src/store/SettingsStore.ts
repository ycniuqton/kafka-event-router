import { DefaultSettings, Theme, type UserSettings } from '../domain';
import { ExternalStore } from './ExternalStore';
import { SafeStorage, StorageKey } from './SafeStorage';

/** User preferences (theme, phase tags), persisted in localStorage and applied to <html>. */
export class SettingsStore extends ExternalStore<UserSettings> {
  constructor() {
    super(SettingsStore.load());
    SettingsStore.applyTheme(this.getState().theme);
  }

  setTheme(theme: Theme): void {
    SafeStorage.set(StorageKey.Theme, theme);
    SettingsStore.applyTheme(theme);
    this.setState({ theme });
  }

  toggleTheme(): void {
    this.setTheme(this.getState().theme === Theme.Dark ? Theme.Light : Theme.Dark);
  }

  setShowPhases(showPhases: boolean): void {
    SafeStorage.set(StorageKey.ShowPhases, String(showPhases));
    this.setState({ showPhases });
  }

  private static load(): UserSettings {
    const theme = SafeStorage.get(StorageKey.Theme);
    const phases = SafeStorage.get(StorageKey.ShowPhases);
    return {
      theme: theme === Theme.Light || theme === Theme.Dark ? theme : DefaultSettings.theme,
      showPhases: phases === null ? DefaultSettings.showPhases : phases === String(true),
    };
  }

  private static applyTheme(theme: Theme): void {
    document.documentElement.dataset.theme = theme;
  }
}
