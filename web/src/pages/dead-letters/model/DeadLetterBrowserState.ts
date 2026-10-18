import type { DeadLetter, DeadLetterPage } from '../../../domain';

/** Loading phase of the browser list. */
export const BrowserLoad = { Idle: 'idle', First: 'first', More: 'more', Failed: 'failed' } as const;
export type BrowserLoad = (typeof BrowserLoad)[keyof typeof BrowserLoad];

type Fields = {
  items: DeadLetter[];
  nextCursor: string | null;
  truncated: boolean;
  checked: string[];
  selectedId: string | null;
  load: BrowserLoad;
  error: string | null;
};

/**
 * Immutable state of the dead-letter browser: loaded rows, cursor, checked rows and the selected row.
 * Every transition returns a new state (pure, unit-tested).
 */
export class DeadLetterBrowserState {
  readonly items: DeadLetter[];
  readonly nextCursor: string | null;
  readonly truncated: boolean;
  readonly checked: string[];
  readonly selectedId: string | null;
  readonly load: BrowserLoad;
  readonly error: string | null;

  private constructor(f: Fields) {
    this.items = f.items;
    this.nextCursor = f.nextCursor;
    this.truncated = f.truncated;
    this.checked = f.checked;
    this.selectedId = f.selectedId;
    this.load = f.load;
    this.error = f.error;
  }

  static initial(selectedId: string | null = null): DeadLetterBrowserState {
    return new DeadLetterBrowserState({ items: [], nextCursor: null, truncated: false, checked: [], selectedId, load: BrowserLoad.First, error: null });
  }

  get hasMore(): boolean {
    return this.nextCursor !== null;
  }

  get loading(): boolean {
    return this.load === BrowserLoad.First || this.load === BrowserLoad.More;
  }

  /** The selected row, else the first one. */
  get selected(): DeadLetter | undefined {
    return this.items.find((d) => d.id === this.selectedId) ?? this.items[0];
  }

  get allChecked(): boolean {
    return this.items.length > 0 && this.checked.length === this.items.length;
  }

  /** Checked rows, in list order. */
  get checkedRows(): DeadLetter[] {
    return this.items.filter((d) => this.checked.includes(d.id));
  }

  /** Starts a reload from the first page (filter changed or refresh); the selection is kept if the row comes back. */
  reloading(): DeadLetterBrowserState {
    return this.with({ items: [], nextCursor: null, truncated: false, checked: [], load: BrowserLoad.First, error: null });
  }

  loadingMore(): DeadLetterBrowserState {
    return this.with({ load: BrowserLoad.More, error: null });
  }

  loaded(page: DeadLetterPage, more: boolean): DeadLetterBrowserState {
    const known = new Set(more ? this.items.map((d) => d.id) : []);
    const items = more ? [...this.items, ...page.items.filter((d) => !known.has(d.id))] : page.items;
    return this.with({ items, nextCursor: page.nextCursor, truncated: page.truncated, load: BrowserLoad.Idle });
  }

  failed(message: string): DeadLetterBrowserState {
    return this.with({ load: BrowserLoad.Failed, error: message });
  }

  select(id: string): DeadLetterBrowserState {
    return this.with({ selectedId: id });
  }

  toggle(id: string): DeadLetterBrowserState {
    return this.with({ checked: this.checked.includes(id) ? this.checked.filter((c) => c !== id) : [...this.checked, id] });
  }

  toggleAll(): DeadLetterBrowserState {
    return this.with({ checked: this.allChecked ? [] : this.items.map((d) => d.id) });
  }

  clearChecked(): DeadLetterBrowserState {
    return this.with({ checked: [] });
  }

  /** Drops resolved rows; a removed selection moves to the row that took its place. */
  without(ids: string[]): DeadLetterBrowserState {
    const gone = new Set(ids);
    const index = this.items.findIndex((d) => d.id === this.selected?.id);
    const items = this.items.filter((d) => !gone.has(d.id));
    const selectedGone = this.selected ? gone.has(this.selected.id) : false;
    const selectedId = selectedGone ? (items[Math.min(index, items.length - 1)]?.id ?? null) : this.selectedId;
    return this.with({ items, selectedId, checked: this.checked.filter((c) => !gone.has(c)) });
  }

  /** Rows whose retry failed again keep their place with the new error message. */
  withFailures(failed: { id: string; message: string }[]): DeadLetterBrowserState {
    const byId = new Map(failed.map((f) => [f.id, f.message]));
    return this.with({ items: this.items.map((d) => (byId.has(d.id) ? { ...d, message: byId.get(d.id) || d.message } : d)) });
  }

  private with(patch: Partial<Fields>): DeadLetterBrowserState {
    return new DeadLetterBrowserState({ ...this, ...patch } as Fields);
  }
}
