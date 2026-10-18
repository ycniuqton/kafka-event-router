import type { CSSProperties } from 'react';
import { services } from '../../../api';
import { Button, Card, CardHeader, Mono, ToneBadge } from '../../../components/ui';
import { RuntimeRefresh, Tone, type RouteRuntime } from '../../../domain';
import { useNow, useServiceQuery } from '../../../store';
import { RuntimeText, RuntimeView } from '../model/RuntimeView';
import styles from './RuntimeCard.module.css';

/** Live runtime of the route from `GET /routes/{id}/runtime`: aggregated state, halt reason and one row per replica. */
export function RuntimeCard({ routeId }: { routeId: string }) {
  const now = useNow(RuntimeRefresh.Ms);
  const { data, error, reload } = useServiceQuery(() => services.runtime.get(routeId), [routeId, now.getTime()]);
  if (!data) {
    return (
      <Card>
        <CardHeader title={RuntimeText.title} meta={error ? RuntimeText.unavailable : RuntimeText.loading}
          actions={error && <Button size="sm" onClick={reload}>{RuntimeText.retry}</Button>} />
      </Card>
    );
  }
  const state = RuntimeView.state(data);
  return (
    <Card>
      <CardHeader title={RuntimeText.title} meta={RuntimeView.meta(data)} actions={<ToneBadge tone={state.tone} dot>{state.label}</ToneBadge>} />
      <HaltNotice runtime={data} now={now} />
      <ReplicaTable runtime={data} now={now} />
    </Card>
  );
}

/** Red banner while the runtime keeps the route halted (resume or a new deploy lifts it). */
function HaltNotice({ runtime, now }: { runtime: RouteRuntime; now: Date }) {
  const halt = RuntimeView.halt(runtime, now);
  if (!halt) return null;
  return (
    <div className={styles.halt} data-tone={Tone.Err} role="alert">
      <strong>{halt.reason}</strong>
      <span className={styles.since}>{halt.since}</span>
      {halt.message && <Mono size="sm" className={styles.message}>{halt.message}</Mono>}
    </div>
  );
}

/** One row per worker replica with its own state, version, lag and report age. */
function ReplicaTable({ runtime, now }: { runtime: RouteRuntime; now: Date }) {
  const rows = RuntimeView.rows(runtime, now);
  if (rows.length === 0) return <div className={styles.empty}>{RuntimeText.noReplicas}</div>;
  const cols = { '--cols': RuntimeView.columns } as CSSProperties;
  return (
    <div role="table">
      <div className={`${styles.row} ${styles.head}`} style={cols} role="row">
        {RuntimeText.columns.map((c) => <span key={c}>{c}</span>)}
      </div>
      {rows.map((r) => (
        <div key={r.worker} className={styles.row} style={cols} role="row" title={r.error ?? undefined}>
          <Mono size="sm" truncate>{r.worker}</Mono>
          <span><ToneBadge tone={r.tone} dot>{r.label}</ToneBadge></span>
          <Mono size="sm">{r.version}</Mono>
          <Mono size="sm">{r.lag}</Mono>
          <span className={styles.muted}>{r.reported}</span>
        </div>
      ))}
    </div>
  );
}
