type Listener = () => void;

/** Minimal observable state container compatible with React's useSyncExternalStore. */
export class ExternalStore<T> {
  private state: T;
  private readonly listeners = new Set<Listener>();

  constructor(initial: T) {
    this.state = initial;
  }

  readonly subscribe = (listener: Listener): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  readonly getState = (): T => this.state;

  /** Shallow-merges a patch (or the result of an updater) and notifies subscribers. */
  setState(patch: Partial<T> | ((current: T) => Partial<T>)): void {
    const next = typeof patch === 'function' ? patch(this.state) : patch;
    this.state = { ...this.state, ...next };
    this.listeners.forEach((listener) => listener());
  }
}
