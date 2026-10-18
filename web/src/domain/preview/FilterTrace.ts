import { PatchPointer } from './PatchPointer';
import type { BranchResult, FilterNodeResult } from './types';

/** Result of one visual filter row for the explained event: null when it was not evaluated. */
export type RowResult = boolean | null;

/** Maps filter node results (JSON Pointers) back to the builder's visual rows ("Why was this filtered?"). */
export class FilterTrace {
  static readonly branchFilter = '/branches/0/filter';
  static readonly childrenToken = 'children';

  /** Filter pointer of branch `index` (routes P2 branches; the first branch is the default). */
  static filterOf(index: number): string {
    return `/branches/${index}/filter`;
  }

  /** Pointer of visual row `index` in a filter with `rowCount` rows (one row is the root itself). */
  static rowPointer(index: number, rowCount: number, filter: string = FilterTrace.branchFilter): string {
    if (rowCount <= 1) return filter;
    return PatchPointer.child(PatchPointer.child(filter, FilterTrace.childrenToken), index);
  }

  static rowResults(branch: BranchResult | undefined, rowCount: number, filter: string = FilterTrace.branchFilter): RowResult[] {
    const byPath = FilterTrace.index(branch?.filter ?? []);
    return Array.from({ length: rowCount }, (_, i) => byPath.get(FilterTrace.rowPointer(i, rowCount, filter))?.result ?? null);
  }

  static node(branch: BranchResult | undefined, pointer: string): FilterNodeResult | undefined {
    return FilterTrace.index(branch?.filter ?? []).get(pointer);
  }

  /** Index of the first row that evaluated to false, or -1. */
  static firstFailed(results: readonly RowResult[]): number {
    return results.findIndex((r) => r === false);
  }

  private static index(nodes: readonly FilterNodeResult[]): Map<string, FilterNodeResult> {
    return new Map(nodes.map((n) => [n.path, n]));
  }
}
