import { Format, Mono, Sparkline, StatusBadge, TableCell, TableRow } from '../../../components/ui';
import { Tone } from '../../../domain';
import type { RouteCommands } from '../model/RouteCommands';
import type { RouteRowModel } from '../model/RouteListModel';
import { RouteRowActions } from './RouteRowActions';
import { RouteSentence } from './RouteSentence';
import styles from './RouteTableRow.module.css';

type RouteTableRowProps = {
  row: RouteRowModel;
  checked: boolean;
  onToggle: (id: string) => void;
  commands: RouteCommands;
  now: Date;
};

/** One table row; clicking it opens the route detail. */
export function RouteTableRow({ row, checked, onToggle, commands, now }: RouteTableRowProps) {
  const { route } = row;
  return (
    <TableRow onClick={() => commands.open(route.id)}>
      <input
        type="checkbox"
        className={styles.check}
        checked={checked}
        onClick={(e) => e.stopPropagation()}
        onChange={() => onToggle(route.id)}
        aria-label={`Select ${route.name}`}
      />
      <StatusBadge status={route.status} />
      <div className={styles.name}>
        <TableCell className={styles.title}>{route.name}</TableCell>
        <div className={styles.owner}>
          {route.owner}
          {row.tags.map((t) => (
            <span key={t} className={styles.tag}>#{t}</span>
          ))}
        </div>
      </div>
      <RouteSentence row={row} />
      <div className={styles.throughput}>
        <Sparkline seed={row.sparkSeed} values={row.sparkValues} flat={row.sparkFlat} tone={row.sparkTone} />
        <Mono tone={Tone.TextSecondary}>{row.throughputText}</Mono>
      </div>
      <Mono tone={Tone.TextSecondary}>{row.matchText}</Mono>
      <Mono tone={row.lagTone}>{row.lagText}</Mono>
      <span className={styles.changed}>{Format.relativeTime(route.updatedAt, now)}</span>
      <RouteRowActions route={route} commands={commands} />
    </TableRow>
  );
}
