import { useMemo } from 'react';
import { Button, ButtonSize, ButtonVariant, SegmentedControl, SegmentSize, TextArea } from '../../../../../components/ui';
import { FilterCombinator, FilterMode, type FilterGroup, type FilterRow } from '../../../../../domain';
import { StepMapper } from '../../../../../domain/routeConfig/StepMapper';
import { FilterRowEditor } from '../../../components/filter/FilterRowEditor';
import { useStudio } from '../../hooks/StudioContext';
import { ConditionFields } from '../../model/ConditionFields';
import { StepChangeLabel } from '../../model/StudioText';
import type { StepSettingsProps } from './StepSettingsProps';
import styles from './RecipeSettings.module.css';

/** Visual conditions or a JS expression, as in the route filter. */
class ConditionModes {
  static readonly options = [
    { value: FilterMode.Visual, label: 'Conditions' },
    { value: FilterMode.Expression, label: 'Expression' },
  ];
}

/** Drop if: drops the event when the condition holds; reuses the route filter's condition row editor. */
export function DropIfSettings({ step, update }: StepSettingsProps) {
  const { model } = useStudio();
  const label = StepChangeLabel.edited(step.kind);
  const condition = step.condition ?? StepMapper.emptyCondition;
  const catalog = useMemo(() => ConditionFields.catalog(model.inputLines, model.samples), [model.inputLines, model.samples]);
  const save = (patch: Partial<FilterGroup>) => update({ condition: { ...condition, ...patch } }, label);
  const setRow = (i: number, row: FilterRow) => save({ rows: condition.rows.map((r, j) => (j === i ? row : r)) });
  const toggle = () => save({ combinator: condition.combinator === FilterCombinator.And ? FilterCombinator.Or : FilterCombinator.And });
  return (
    <>
      <div className={styles.start}>
        <SegmentedControl size={SegmentSize.Xs} options={ConditionModes.options} value={condition.mode} onChange={(mode) => save({ mode })} ariaLabel="Condition mode" />
      </div>
      {condition.mode === FilterMode.Expression ? (
        <TextArea mono sunken value={condition.expression} placeholder="event.value.amount < 0" onChange={(e) => save({ expression: e.target.value })} aria-label="Drop condition expression" />
      ) : (
        <>
          {condition.rows.map((row, i) => (
            <div key={i} className={styles.condition}>
              {i > 0 && (
                <button type="button" className={styles.combinator} onClick={toggle} title="Toggle AND / OR">
                  {condition.combinator}
                </button>
              )}
              <FilterRowEditor row={row} catalog={catalog} result={null} onReplace={(next) => setRow(i, next)} onDelete={() => save({ rows: condition.rows.filter((_, j) => j !== i) })} />
            </div>
          ))}
          <div className={styles.start}>
            <Button size={ButtonSize.Sm} variant={ButtonVariant.Ghost} onClick={() => save({ rows: [...condition.rows, catalog.newRow()] })}>
              + Add condition
            </Button>
          </div>
        </>
      )}
    </>
  );
}
