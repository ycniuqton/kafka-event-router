import type { RouteCommands } from '../model/RouteCommands';
import type { RouteRowModel } from '../model/RouteListModel';
import type { RouteSelection } from '../model/RouteSelection';
import { RouteCards } from './RouteCards';
import { RoutesEmpty } from './RoutesEmpty';
import { RoutesTable } from './RoutesTable';

type RoutesBodyProps = {
  rows: RouteRowModel[];
  hasRoutes: boolean;
  cards: boolean;
  selection: RouteSelection;
  onToggle: (id: string) => void;
  commands: RouteCommands;
  now: Date;
};

/** Table, cards or the empty state, depending on the filtered rows and the chosen view. */
export function RoutesBody({ rows, hasRoutes, cards, selection, onToggle, commands, now }: RoutesBodyProps) {
  if (!rows.length) return <RoutesEmpty hasRoutes={hasRoutes} />;
  if (cards) return <RouteCards rows={rows} commands={commands} />;
  return <RoutesTable rows={rows} selection={selection} onToggle={onToggle} commands={commands} now={now} />;
}
