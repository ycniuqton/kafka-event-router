import type { AppStore } from './AppStore';

/** Shows one transient message at a time at the bottom of the screen. */
export class ToastController {
  static readonly durationMs = 2800;
  private readonly store: AppStore;
  private timer: ReturnType<typeof setTimeout> | undefined;
  private sequence = 0;

  constructor(store: AppStore) {
    this.store = store;
  }

  show(message: string): void {
    clearTimeout(this.timer);
    this.sequence += 1;
    this.store.setState({ toast: { id: this.sequence, message } });
    this.timer = setTimeout(() => this.dismiss(), ToastController.durationMs);
  }

  dismiss(): void {
    clearTimeout(this.timer);
    this.store.setState({ toast: null });
  }
}
