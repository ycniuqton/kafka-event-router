import { Fragment } from 'react';
import { Button, ButtonSize } from '../../../../components/ui';
import { FilterGroups, type FilterGroup } from '../../../../domain';
import type { FilterFieldCatalog } from '../../model/FilterFieldCatalog';
import { FilterGroupEditor } from '../../model/FilterGroupEditor';
import type { GroupResults } from '../../model/FilterWhy';
import { FilterRowEditor } from './FilterRowEditor';
import styles from './FilterRowList.module.css';

export type ConditionGroupProps = {
  group: FilterGroup;
  depth: number;
  catalog: FilterFieldCatalog;
  results: GroupResults | null;
  onChange: (group: FilterGroup, change: string) => void;
  onRemove?: () => void;
};

/** One AND/OR group: its condition rows and nested sub-groups (recursive), with add / remove actions. */
export function ConditionGroup({ group, depth, catalog, results, onChange, onRemove }: ConditionGroupProps) {
  const subGroups = FilterGroups.children(group);
  const toggle = () => onChange(FilterGroupEditor.toggleCombinator(group), 'Combinator changed');
  const joiner = (i: number) =>
    i > 0 && (
      <button type="button" className={styles.comb} title="Toggle AND / OR" onClick={toggle}>
        {group.combinator}
      </button>
    );
  return (
    <div className={depth > 0 ? styles.nested : styles.list}>
      {group.rows.map((row, i) => (
        <Fragment key={`r${i}`}>
          {joiner(i)}
          <FilterRowEditor
            row={row}
            catalog={catalog}
            result={results?.rows[i]}
            onReplace={(next, change) => onChange(FilterGroupEditor.replaceRow(group, i, next), change)}
            onDelete={() => onChange(FilterGroupEditor.removeRow(group, i), 'Removed filter condition')}
          />
        </Fragment>
      ))}
      {subGroups.map((sub, i) => (
        <Fragment key={`g${i}`}>
          {joiner(group.rows.length + i)}
          <ConditionGroup
            group={sub}
            depth={depth + 1}
            catalog={catalog}
            results={results?.groups[i] ?? null}
            onChange={(next, change) =>
              onChange(
                FilterGroupEditor.update(group, [i], () => next),
                change,
              )
            }
            onRemove={() => onChange(FilterGroupEditor.removeGroup(group, i), 'Removed filter group')}
          />
        </Fragment>
      ))}
      <GroupActions group={group} depth={depth} catalog={catalog} onChange={onChange} onRemove={onRemove} />
    </div>
  );
}

type GroupActionsProps = Pick<ConditionGroupProps, 'group' | 'depth' | 'catalog' | 'onChange' | 'onRemove'>;

/** "+ Add condition", "+ Add group" (up to the nesting limit) and "Remove group" for sub-groups. */
function GroupActions({ group, depth, catalog, onChange, onRemove }: GroupActionsProps) {
  return (
    <div className={styles.actions}>
      <Button size={ButtonSize.Md} className={styles.add} onClick={() => onChange(FilterGroupEditor.addRow(group, catalog.newRow()), 'Added filter condition')}>
        + Add condition
      </Button>
      {depth + 1 < FilterGroups.maxDepth && (
        <Button
          size={ButtonSize.Md}
          className={styles.group}
          onClick={() => onChange(FilterGroupEditor.addGroup(group, catalog.newRow()), 'Added filter group')}
        >
          + Add group
        </Button>
      )}
      {onRemove && (
        <Button size={ButtonSize.Md} className={styles.group} onClick={onRemove}>
          Remove group
        </Button>
      )}
    </div>
  );
}
