import { BranchResults, FilterGroups, FilterTrace, type FilterGroup, type RowResult, type RunResult, type SampleRecord } from '../../../domain';
import { FilterTreeModel, type FilterPointers } from '../../../domain/routeConfig/FilterTreeModel';

/** The edited branch: position (for trace pointers) and id (null for a single-branch route). */
export type BranchAt = { index: number; id: string | null };

/** Per-condition results of the explained sample, mirroring the nested group (null = not evaluated). */
export type GroupResults = { rows: RowResult[]; groups: GroupResults[] };

/** "Why was this filtered?" for nested groups: maps the preview trace (JSON Pointers) onto every visual row. */
export class FilterWhy {
  static results(filter: FilterGroup, records: SampleRecord[], run: RunResult | null, index: number | null, at: BranchAt = FilterWhy.firstBranch): GroupResults | null {
    if (index === null || !run) return null;
    const seq = records[index]?.seq;
    const result = run.results.find((r) => r.seq === seq);
    const branch = result ? BranchResults.view(result, at.id).branch : undefined;
    if (!branch) return null;
    const byPath = new Map(branch.filter.map((n) => [n.path, n.result]));
    return FilterWhy.map(filter, FilterTreeModel.pointers(filter, FilterTrace.filterOf(at.index)), byPath);
  }

  static readonly firstBranch: BranchAt = { index: 0, id: null };

  static none(group: FilterGroup): GroupResults {
    return { rows: group.rows.map(() => null), groups: FilterGroups.children(group).map((g) => FilterWhy.none(g)) };
  }

  private static map(group: FilterGroup, pointers: FilterPointers, byPath: Map<string, boolean>): GroupResults {
    return {
      rows: pointers.rows.map((p) => byPath.get(p) ?? null),
      groups: FilterGroups.children(group).map((g, i) => FilterWhy.map(g, pointers.groups[i], byPath)),
    };
  }
}
