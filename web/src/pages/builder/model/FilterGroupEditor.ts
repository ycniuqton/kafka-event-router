import { FilterCombinator, FilterGroups, type FilterGroup, type FilterRow } from '../../../domain';

/** Address of a (sub-)group: indexes into `groups` from the root filter. */
export type GroupPath = readonly number[];

/** Immutable edits of a nested filter group at a path (rows, combinator, sub-groups). */
export class FilterGroupEditor {
  static at(root: FilterGroup, path: GroupPath): FilterGroup {
    return path.reduce<FilterGroup>((g, i) => FilterGroups.children(g)[i], root);
  }

  /** Returns a copy of `root` with the group at `path` replaced by `update(group)`. */
  static update(root: FilterGroup, path: GroupPath, update: (group: FilterGroup) => FilterGroup): FilterGroup {
    if (!path.length) return update(root);
    const [head, ...rest] = path;
    const groups = FilterGroups.children(root).map((g, i) => (i === head ? FilterGroupEditor.update(g, rest, update) : g));
    return { ...root, groups };
  }

  static replaceRow(group: FilterGroup, index: number, row: FilterRow): FilterGroup {
    return { ...group, rows: group.rows.map((r, i) => (i === index ? row : r)) };
  }

  static removeRow(group: FilterGroup, index: number): FilterGroup {
    return { ...group, rows: group.rows.filter((_, i) => i !== index) };
  }

  static addRow(group: FilterGroup, row: FilterRow): FilterGroup {
    return { ...group, rows: [...group.rows, row] };
  }

  static toggleCombinator(group: FilterGroup): FilterGroup {
    return { ...group, combinator: group.combinator === FilterCombinator.And ? FilterCombinator.Or : FilterCombinator.And };
  }

  static addGroup(group: FilterGroup, row: FilterRow): FilterGroup {
    return { ...group, groups: [...FilterGroups.children(group), FilterGroups.newGroup(group, row)] };
  }

  static removeGroup(group: FilterGroup, index: number): FilterGroup {
    return { ...group, groups: FilterGroups.children(group).filter((_, i) => i !== index) };
  }
}
