/** Immutable set of selected route ids for bulk actions. */
export class RouteSelection {
  static readonly empty = new RouteSelection(new Set());

  private readonly ids: ReadonlySet<string>;

  private constructor(ids: ReadonlySet<string>) {
    this.ids = ids;
  }

  has(id: string): boolean {
    return this.ids.has(id);
  }

  get size(): number {
    return this.ids.size;
  }

  toArray(): string[] {
    return [...this.ids];
  }

  toggle(id: string): RouteSelection {
    const next = new Set(this.ids);
    if (!next.delete(id)) next.add(id);
    return new RouteSelection(next);
  }
}
