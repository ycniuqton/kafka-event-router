import { Button, DataTable, EmptyState, TableHead } from '../../../components/ui';
import { Can } from '../../../access';
import { Permission, type Connection } from '../../../domain';
import type { ConnectionRowView } from '../model/ConnectionRowModel';
import { ConnectionRow } from './ConnectionRow';

/** Grid columns of the design's connections table, scrolling below 1000px. */
const ConnectionsTableLayout = {
  columns: 'minmax(150px,1fr) 80px minmax(220px,1.4fr) 130px 150px 70px 100px 230px',
  minWidth: 1000,
  headers: ['Name', 'Env', 'Bootstrap servers', 'Security', 'Status', 'Used by', 'Owner', ''],
} as const;

export type ConnectionsTableProps = {
  rows: ConnectionRowView[];
  filtered: boolean;
  onCreate: () => void;
  onTest: (id: string) => void;
  onBrowse: (id: string) => void;
  onEdit: (id: string) => void;
  onDelete: (connection: Connection) => void;
};

/** Connections list: name, env, servers, security, status, used-by, owner and row actions (spec §5.1). */
export function ConnectionsTable({ rows, filtered, onCreate, onTest, onBrowse, onEdit, onDelete }: ConnectionsTableProps) {
  if (!rows.length && filtered) return <EmptyState title="No connections match these filters." description="Clear the search or pick another environment or status." />;
  if (!rows.length) {
    return (
      <EmptyState
        title="No connections yet."
        description="A connection is entered once and reused by every route. Add one to start routing events."
        action={
          <Can permission={Permission.ConnectionWrite}>
            <Button onClick={onCreate}>+ New connection</Button>
          </Can>
        }
      />
    );
  }
  return (
    <DataTable columns={ConnectionsTableLayout.columns} minWidth={ConnectionsTableLayout.minWidth}>
      <TableHead>
        {ConnectionsTableLayout.headers.map((h, i) => (
          <span key={i}>{h}</span>
        ))}
      </TableHead>
      {rows.map((row) => (
        <ConnectionRow key={row.connection.id} row={row} onTest={onTest} onBrowse={onBrowse} onEdit={onEdit} onDelete={onDelete} />
      ))}
    </DataTable>
  );
}
