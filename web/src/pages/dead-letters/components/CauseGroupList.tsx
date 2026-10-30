import { Button, Card, EmptyState, Format, Mono } from '../../../components/ui';
import { Can } from '../../../access';
import { DeadLetterErrorClassMeta, Permission, type DeadLetterGroup } from '../../../domain';
import { useNow } from '../../../store';
import type { DeadLetterRouteNames } from '../model/DeadLetterBrowserModel';
import type { CauseGroupsState } from '../model/useCauseGroups';
import { ErrorClassBadge } from './ErrorClassBadge';
import { TrendBars } from './TrendBars';
import styles from './CauseGroupList.module.css';

export type CauseGroupListProps = {
  groups: CauseGroupsState;
  names: DeadLetterRouteNames;
  showRoute: boolean;
  onShowEvents: (group: DeadLetterGroup) => void;
  onDryRun: (group: DeadLetterGroup) => void;
  onReplay: (group: DeadLetterGroup) => void;
};

/** Distinct causes of the filtered dead letters, biggest first: whose fault, how often, since when, and what to do. */
export function CauseGroupList({ groups, names, showRoute, ...actions }: CauseGroupListProps) {
  const { page, loading, error } = groups;
  if (error) return <EmptyState title="Could not group the dead letters" description={error} compact />;
  if (!loading && !page.items.length) return <EmptyState title="No causes" description="No unresolved dead letters match this filter." compact />;
  return (
    <Card padding="none" className={styles.list}>
      <div className={styles.summary}>
        {loading ? 'Grouping dead letters…' : `${page.items.length} causes over ${page.scanned} events${page.truncated || page.approximate ? ' (scan stopped at its budget, counts are approximate)' : ''}`}
      </div>
      {page.items.map((g) => (
        <CauseGroupRow key={g.fingerprint} group={g} routeName={names.of(g.routeId, g.routeName)} showRoute={showRoute} {...actions} />
      ))}
    </Card>
  );
}

type RowProps = Omit<CauseGroupListProps, 'groups' | 'names'> & { group: DeadLetterGroup; routeName: string };

function CauseGroupRow({ group: g, routeName, showRoute, onShowEvents, onDryRun, onReplay }: RowProps) {
  const now = useNow();
  const versions = g.versions.length ? ` · v${g.versions.join(', v')}` : '';
  return (
    <div className={styles.row} data-class={g.errorClass}>
      <div className={styles.count}>{Format.number(g.count)}</div>
      <div className={styles.body}>
        <div className={styles.title}>
          <ErrorClassBadge errorClass={g.errorClass} />
          <span className={styles.kind}>{g.kind}</span>
          <span className={styles.meta}>
            {showRoute ? `${routeName} · ` : ''}
            {g.step}
            {versions}
          </span>
        </div>
        <Mono className={styles.pattern}>{g.pattern}</Mono>
        <div className={styles.meta}>
          first {Format.relativeTime(g.firstSeenAt, now)} · last {Format.relativeTime(g.lastSeenAt, now)} · e.g. <span className={styles.sample}>{g.sample.message}</span>
        </div>
      </div>
      <TrendBars trend={g.trend} tone={DeadLetterErrorClassMeta[g.errorClass].tone} />
      <div className={styles.actions}>
        <Button size="sm" variant="ghost" onClick={() => onShowEvents(g)}>
          Show events
        </Button>
        <Can permission={Permission.DlqAct}>
          <Button size="sm" onClick={() => onDryRun(g)}>
            Dry-run
          </Button>
          <Button size="sm" onClick={() => onReplay(g)}>
            Replay group
          </Button>
        </Can>
      </div>
    </div>
  );
}
