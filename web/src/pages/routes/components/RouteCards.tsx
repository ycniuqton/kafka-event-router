import type { RouteCommands } from '../model/RouteCommands';
import type { RouteRowModel } from '../model/RouteListModel';
import { RouteCard } from './RouteCard';
import styles from './RouteCards.module.css';

/** Responsive grid of route cards (also the phone layout). */
export function RouteCards({ rows, commands }: { rows: RouteRowModel[]; commands: RouteCommands }) {
  return (
    <div className={styles.grid}>
      {rows.map((row) => (
        <RouteCard key={row.route.id} row={row} commands={commands} />
      ))}
    </div>
  );
}
