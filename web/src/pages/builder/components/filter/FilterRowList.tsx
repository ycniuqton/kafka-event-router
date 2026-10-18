import { FilterGroups, type FilterGroup } from '../../../../domain';
import type { DraftEditor } from '../../model/DraftSession';
import type { FilterFieldCatalog } from '../../model/FilterFieldCatalog';
import type { GroupResults } from '../../model/FilterWhy';
import { ConditionGroup } from './ConditionGroup';
import styles from './FilterRowList.module.css';

export type FilterRowListProps = {
  filter: FilterGroup;
  catalog: FilterFieldCatalog;
  editor: DraftEditor;
  /** Per-condition results of the "Why?" sample (nested like the groups), or null when nothing is explained. */
  results: GroupResults | null;
};

/** Visual filter: the root condition group (nested AND/OR groups), or a hint when it is empty. */
export function FilterRowList({ filter, catalog, editor, results }: FilterRowListProps) {
  const replace = (group: FilterGroup, change: string) =>
    editor.updateFilter(
      () => ({
        combinator: group.combinator,
        rows: group.rows,
        groups: FilterGroups.children(group),
      }),
      change,
    );
  return (
    <div className={styles.list}>
      {FilterGroups.isEmpty(filter) && (
        <div className={styles.empty}>No conditions — every event passes. Add a row, or click a value in the sample panel to filter on it.</div>
      )}
      <ConditionGroup group={filter} depth={0} catalog={catalog} results={results} onChange={replace} />
    </div>
  );
}
