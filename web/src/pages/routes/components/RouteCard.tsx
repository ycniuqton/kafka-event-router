import { Card, Mono, StatusBadge } from '../../../components/ui';
import { Tone } from '../../../domain';
import type { RouteCommands } from '../model/RouteCommands';
import type { RouteRowModel } from '../model/RouteListModel';
import { RouteRowActions } from './RouteRowActions';
import styles from './RouteCard.module.css';

type RouteCardProps = { row: RouteRowModel; commands: RouteCommands };

/** Card view of a route: source, filter · transform, destination and live numbers. */
export function RouteCard({ row, commands }: RouteCardProps) {
  const { route } = row;
  return (
    <Card interactive className={styles.card} onClick={() => commands.open(route.id)}>
      <div className={styles.head}>
        <StatusBadge status={route.status} dot={false} />
        <span className={styles.name}>{route.name}</span>
      </div>
      <div className={styles.flow}>
        <div className={styles.block}>{row.sourceText}</div>
        <div className={`${styles.block} ${styles.middle}`}>
          {route.filterSummary} · {row.transformLabel}
        </div>
        <div className={styles.block}>{row.destinationText}</div>
      </div>
      <div className={styles.foot}>
        <div className={styles.metrics}>
          <Mono tone={Tone.TextSecondary}>{row.throughputText}</Mono>
          <Mono tone={Tone.TextSecondary}>match {row.matchText}</Mono>
          <Mono tone={row.lagTone}>lag {row.lagText}</Mono>
        </div>
        <RouteRowActions route={route} commands={commands} compact />
      </div>
    </Card>
  );
}
