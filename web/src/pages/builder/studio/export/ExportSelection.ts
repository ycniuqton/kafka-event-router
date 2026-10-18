import { TransformStepKind, type TransformStep } from '../../../../domain';

/** The steps picked for "Export to JS": recipe steps only, kept in pipeline order. */
export class ExportSelection {
  private readonly ids: ReadonlySet<number>;

  constructor(ids: readonly number[]) {
    this.ids = new Set(ids);
  }

  static selectable(step: TransformStep): boolean {
    return step.kind !== TransformStepKind.Js;
  }

  has(id: number): boolean {
    return this.ids.has(id);
  }

  toggle(id: number): number[] {
    return this.ids.has(id) ? [...this.ids].filter((x) => x !== id) : [...this.ids, id];
  }

  /** Selected steps in pipeline order. */
  steps(all: readonly TransformStep[]): TransformStep[] {
    return all.filter((s) => this.ids.has(s.id) && ExportSelection.selectable(s));
  }

  get idSet(): ReadonlySet<number> {
    return this.ids;
  }

  get size(): number {
    return this.ids.size;
  }
}
