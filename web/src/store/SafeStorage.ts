/** localStorage access that never throws (private mode, blocked storage, SSR). */
export class SafeStorage {
  static get(key: string): string | null {
    try {
      return window.localStorage.getItem(key);
    } catch {
      return null;
    }
  }

  /** Returns false when storage is unavailable; the value then lives only in memory. */
  static set(key: string, value: string): boolean {
    try {
      window.localStorage.setItem(key, value);
      return true;
    } catch {
      return false;
    }
  }
}

export const StorageKey = { Theme: 'er.theme', ShowPhases: 'er.showPhases' } as const;
