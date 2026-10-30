import { Button, DataTable, EmptyState, EnvTag, HealthDot, Mono, TableCell, TableHead, TableRow } from '../../../components/ui';
import { Can } from '../../../access';
import { Permission, Tone, type LookupSource } from '../../../domain';
import type { LookupSourceRowView } from './model/LookupSourceRows';
import styles from './LookupSources.module.css';

/** Grid columns of the lookup sources table, scrolling below 960px. */
const LookupTableLayout = {
  columns: 'minmax(140px,1fr) 90px 80px minmax(200px,1.4fr) 150px 70px 110px 170px',
  minWidth: 960,
  headers: ['Name', 'Kind', 'Env', 'Target', 'Status', 'Used by', 'Owner', ''],
} as const;

export type LookupSourcesTableProps = {
  rows: LookupSourceRowView[];
  filtered: boolean;
  onCreate: () => void;
  onTest: (source: LookupSource) => void;
  onEdit: (source: LookupSource) => void;
  onDelete: (source: LookupSource) => void;
};

/** Lookup sources list (LS-01): name, kind, env, target, last test, used-by, owner and row actions. */
export function LookupSourcesTable({ rows, filtered, onCreate, onTest, onEdit, onDelete }: LookupSourcesTableProps) {
  if (!rows.length && filtered) return <EmptyState title="No lookup sources match these filters." description="Clear the search or pick another kind." />;
  if (!rows.length) {
    return (
      <EmptyState title="No lookup sources yet." description="A lookup source is an API, Redis, Postgres database or static table that lookup steps use to enrich events."
        action={<Can permission={Permission.ConnectionWrite}><Button onClick={onCreate}>+ New lookup source</Button></Can>} />
    );
  }
  return (
    <DataTable columns={LookupTableLayout.columns} minWidth={LookupTableLayout.minWidth}>
      <TableHead>
        {LookupTableLayout.headers.map((h, i) => <span key={i}>{h}</span>)}
      </TableHead>
      {rows.map((row) => <LookupSourceRow key={row.source.id} row={row} onTest={onTest} onEdit={onEdit} onDelete={onDelete} />)}
    </DataTable>
  );
}

type LookupSourceRowProps = Omit<LookupSourcesTableProps, 'rows' | 'filtered' | 'onCreate'> & { row: LookupSourceRowView };

/** One source; Delete is disabled while routes use it (tooltip lists them); writes need connection.write on the owner team. */
function LookupSourceRow({ row, onTest, onEdit, onDelete }: LookupSourceRowProps) {
  const s = row.source;
  const inUse = s.usedBy.count > 0;
  return (
    <TableRow>
      <Mono size="md" medium truncate>{s.name}</Mono>
      <span className={styles.secondary}>{row.kindLabel}</span>
      <span className={styles.env}><EnvTag env={s.envTag} /></span>
      <TableCell title={row.target}><Mono tone={Tone.TextSecondary} truncate>{row.target}</Mono></TableCell>
      <div>
        <span className={styles.status} title={row.statusTitle}><HealthDot tone={row.statusTone} label={row.statusLabel} /></span>
        <div className={styles.checked}>checked {row.checked}</div>
      </div>
      <span title={inUse ? row.deleteTitle : undefined}><Mono tone={Tone.TextSecondary}>{s.usedBy.count}</Mono></span>
      <span className={styles.secondary}>{s.owner.teamName}</span>
      <div className={styles.actions}>
        <Can permission={Permission.ConnectionWrite} teamId={s.owner.teamId}>
          <Button size="sm" onClick={() => onTest(s)}>Test</Button>
          <Button size="sm" onClick={() => onEdit(s)}>Edit</Button>
          <span className={styles.deleteWrap} title={row.deleteTitle}>
            <Button size="sm" variant="danger" disabled={inUse} onClick={() => onDelete(s)}>Delete</Button>
          </span>
        </Can>
      </div>
    </TableRow>
  );
}
