import { Button, Select } from '../../../components/ui';
import { FlowFilterAll, FlowFilterManager, type FlowFilterContext, type FlowFilterKey, type FlowFilterValues } from '../model/FlowFilters';
import styles from './FlowMapFilters.module.css';

export type FlowMapFiltersProps = {
  values: FlowFilterValues;
  context: FlowFilterContext;
  onChange: (key: FlowFilterKey, value: string) => void;
  onReset: () => void;
};

/** Connection / team / tag selects above the flow map (spec §4). */
export function FlowMapFilters({ values, context, onChange, onReset }: FlowMapFiltersProps) {
  return (
    <div className={styles.bar}>
      {FlowFilterManager.dimensions.map((d) => (
        <Select key={d.key} controlSize="sm" aria-label={d.allLabel} value={values[d.key]} onChange={(e) => onChange(d.key, e.target.value)}>
          <option value={FlowFilterAll}>{d.allLabel}</option>
          {d.options(context).map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
        </Select>
      ))}
      {FlowFilterManager.isFiltered(values) && (
        <Button variant="ghost" size="sm" onClick={onReset}>
          Clear filters
        </Button>
      )}
    </div>
  );
}
