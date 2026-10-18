import { Chip, SegmentedControl, TextInput } from '../../../components/ui';
import { ConnectionStatusMeta, EnvMeta, type Connection } from '../../../domain';
import { ConnectionFilter, FilterAll, type ConnectionFilterState } from '../model/ConnectionFilter';
import styles from './ConnectionFilters.module.css';

/** Labels of the "All" filter options. */
const FilterText = { all: 'All', search: 'Search name, description or broker' } as const;

export type ConnectionFiltersProps = { connections: Connection[]; value: ConnectionFilterState; onChange: (value: ConnectionFilterState) => void };

/** Search box, env segmented control and status chips with counts above the connections table. */
export function ConnectionFilters({ connections, value, onChange }: ConnectionFiltersProps) {
  const envOptions = ConnectionFilter.envOptions.map((env) => ({ value: env, label: env === FilterAll ? FilterText.all : EnvMeta[env].label }));
  return (
    <div className={styles.bar}>
      <TextInput className={styles.search} controlSize="md" sunken value={value.q} placeholder={FilterText.search} aria-label={FilterText.search} onChange={(e) => onChange({ ...value, q: e.target.value })} />
      <SegmentedControl size="sm" options={envOptions} value={value.env} onChange={(env) => onChange({ ...value, env })} ariaLabel="Environment filter" />
      <div className={styles.chips}>
        {ConnectionFilter.statusOptions.map((status) => (
          <Chip key={status} active={value.status === status} count={ConnectionFilter.statusCount(connections, value, status)} onClick={() => onChange({ ...value, status })}>
            {status === FilterAll ? FilterText.all : ConnectionStatusMeta[status].label}
          </Chip>
        ))}
      </div>
    </div>
  );
}
