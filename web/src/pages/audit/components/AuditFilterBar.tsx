import { Chip, Select, TextInput } from '../../../components/ui';
import { AuditActions, AuditCategory, AuditObjectType, type AuditActionInfo, type AuditActor, type Team } from '../../../domain';
import { AuditFilter, AuditTimeRange, type AuditFilterState } from '../model/AuditFilter';
import { AuditPresets } from '../model/AuditPresets';
import styles from './AuditFilterBar.module.css';

export type AuditFilterBarProps = {
  filter: AuditFilterState;
  actors: AuditActor[];
  actions: AuditActionInfo[];
  teams: Team[];
  onChange: (filter: AuditFilterState) => void;
};

/** Category headings of the action select. */
const CategoryLabel: Record<AuditCategory, string> = {
  [AuditCategory.Change]: 'Changes',
  [AuditCategory.Access]: 'Data access',
  [AuditCategory.Admin]: 'Administration',
};

/** Filters above the audit table: text, actor, action, object type, team, time range, presets, sensitive only. */
export function AuditFilterBar({ filter, actors, actions, teams, onChange }: AuditFilterBarProps) {
  const set = (patch: Partial<AuditFilterState>) => onChange({ ...filter, ...patch });
  return (
    <div className={styles.bar}>
      <TextInput className={styles.search} value={filter.text} placeholder="Filter by action or object name" aria-label="Filter audit log"
        onChange={(e) => set({ text: e.target.value })} />
      <Select className={styles.select} value={filter.actorId} aria-label="Who" onChange={(e) => set({ actorId: e.target.value })}>
        <option value={AuditFilter.any}>Everyone</option>
        {actors.map((a) => <option key={a.id} value={a.id}>{a.name}</option>)}
      </Select>
      <ActionSelect value={filter.action} actions={actions} onChange={(action) => set({ action })} />
      <Select className={styles.select} value={filter.objectType} aria-label="Object type" onChange={(e) => set({ objectType: e.target.value })}>
        <option value={AuditFilter.any}>All objects</option>
        {Object.values(AuditObjectType).map((t) => <option key={t} value={t}>{t.replace(/_/g, ' ')}</option>)}
      </Select>
      <Select className={styles.select} value={filter.teamId} aria-label="Team" onChange={(e) => set({ teamId: e.target.value })}>
        <option value={AuditFilter.any}>All teams</option>
        {teams.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
      </Select>
      <Select className={styles.select} value={filter.range} aria-label="Time range" onChange={(e) => set({ range: e.target.value as AuditTimeRange })}>
        {Object.values(AuditTimeRange).map((r) => <option key={r} value={r}>{AuditFilter.rangeLabels[r]}</option>)}
      </Select>
      {filter.range === AuditTimeRange.Custom && <CustomRange filter={filter} onChange={set} />}
      {AuditPresets.all.map((p) => (
        <span key={p.id} title={p.title}>
          <Chip active={AuditPresets.active(filter, p)} onClick={() => onChange(AuditPresets.toggle(filter, p))}>
            {p.label}
          </Chip>
        </span>
      ))}
      <Chip active={filter.sensitiveOnly} onClick={() => set({ sensitiveOnly: !filter.sensitiveOnly })}>
        Sensitive only
      </Chip>
    </div>
  );
}

function ActionSelect({ value, actions, onChange }: { value: string; actions: AuditActionInfo[]; onChange: (action: string) => void }) {
  return (
    <Select className={styles.select} value={value} aria-label="Action" onChange={(e) => onChange(e.target.value)}>
      <option value={AuditFilter.any}>All actions</option>
      {Object.values(AuditCategory).map((category) => (
        <optgroup key={category} label={CategoryLabel[category]}>
          {actions.filter((a) => a.category === category).map((a) => (
            <option key={a.action} value={a.action}>{AuditActions.label(a.action)}</option>
          ))}
        </optgroup>
      ))}
    </Select>
  );
}

function CustomRange({ filter, onChange }: { filter: AuditFilterState; onChange: (patch: Partial<AuditFilterState>) => void }) {
  return (
    <span className={styles.custom}>
      <TextInput type="datetime-local" value={filter.customFrom} aria-label="From" onChange={(e) => onChange({ customFrom: e.target.value })} />
      to
      <TextInput type="datetime-local" value={filter.customTo} aria-label="To" onChange={(e) => onChange({ customTo: e.target.value })} />
    </span>
  );
}
