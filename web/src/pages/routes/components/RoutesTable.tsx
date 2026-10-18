import { DataTable, TableHead } from '../../../components/ui';
import type { RouteCommands } from '../model/RouteCommands';
import type { RouteRowModel } from '../model/RouteListModel';
import type { RouteSelection } from '../model/RouteSelection';
import { RouteTableRow } from './RouteTableRow';

/** Column template and minimum width of the routes table (design, plus room for "View metrics"). */
const RoutesTableLayout = {
  columns: '32px 88px minmax(150px,1fr) minmax(260px,2fr) 110px 56px 64px 88px 200px',
  minWidth: 1160,
} as const;

type RoutesTableProps = {
  rows: RouteRowModel[];
  selection: RouteSelection;
  onToggle: (id: string) => void;
  commands: RouteCommands;
  now: Date;
};

/** Dense routes table: one sentence-like row per route. */
export function RoutesTable({ rows, selection, onToggle, commands, now }: RoutesTableProps) {
  return (
    <DataTable columns={RoutesTableLayout.columns} minWidth={RoutesTableLayout.minWidth}>
      <TableHead>
        <span />
        <span>Status</span>
        <span>Route</span>
        <span>Source → filter → transform → destination</span>
        <span>Throughput</span>
        <span>Match</span>
        <span>Lag</span>
        <span>Changed</span>
        <span />
      </TableHead>
      {rows.map((row) => (
        <RouteTableRow
          key={row.route.id}
          row={row}
          checked={selection.has(row.route.id)}
          onToggle={onToggle}
          commands={commands}
          now={now}
        />
      ))}
    </DataTable>
  );
}
