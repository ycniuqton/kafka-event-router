import { FieldPath } from '../../../../domain';

/** Immutable set of collapsed branch paths of a JSON tree. */
export class CollapsedPaths {
  private readonly paths: ReadonlySet<string>;

  constructor(paths: Iterable<string> = []) {
    this.paths = new Set(paths);
  }

  has(path: string): boolean {
    return this.paths.has(path);
  }

  /** True when an ancestor of the path is collapsed. */
  hides(path: string): boolean {
    return [...this.paths].some((p) => path.startsWith(`${p}${FieldPath.separator}`));
  }

  toggle(path: string): CollapsedPaths {
    const next = new Set(this.paths);
    if (!next.delete(path)) next.add(path);
    return new CollapsedPaths(next);
  }
}
