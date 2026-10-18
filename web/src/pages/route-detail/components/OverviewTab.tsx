import { useMemo, useState } from 'react';
import { services } from '../../../api';
import { Button, EmptyState, SegmentedControl } from '../../../components/ui';
import type { Route } from '../../../domain';
import { useServiceQuery } from '../../../store';
import { OverviewSeriesView } from '../model/OverviewSeriesView';
import { RouteDetailModel, TimeRange, TimeRangeOptions } from '../model/RouteDetailModel';
import { OverviewCharts } from './OverviewCharts';
import { OverviewKpis } from './OverviewKpis';
import { PipelineFlow } from './PipelineFlow';
import { RuntimeCard } from './RuntimeCard';
import styles from './OverviewTab.module.css';

/** Texts of the overview's loading and metrics-unavailable (503) states. */
const OverviewText = {
  loading: 'Loading metrics…',
  unavailableTitle: 'Metrics are temporarily unavailable',
  unavailableBody: 'The metrics store did not answer. Route status and settings still work.',
  retry: 'Retry',
  emptyTitle: 'No metrics in this range yet',
  emptyBody: 'Charts fill in once the route has processed events in the selected time range.',
} as const;

/** Overview: range selector, pipeline flow with counts, KPI tiles and charts from `GET /routes/{id}/metrics` (spec §10.1). */
export function OverviewTab({ route }: { route: Route }) {
  const [range, setRange] = useState<TimeRange>(TimeRange.H1);
  const model = new RouteDetailModel(route);
  const { data, error, reload } = useServiceQuery(() => services.metrics.routeMetrics(route.id, range), [route.id, range]);
  const charts = useMemo(() => (data ? OverviewSeriesView.chartData(data) : null), [data]);
  const counts = data ? OverviewSeriesView.counts(data) : null;
  const branches = data ? OverviewSeriesView.branchCounts(data) : null;
  const empty = !!data && OverviewSeriesView.isEmpty(data);
  return (
    <div className={styles.tab}>
      <div className={styles.range}>
        <SegmentedControl options={TimeRangeOptions} value={range} onChange={setRange} mono ariaLabel="Time range" />
      </div>
      {branches ? (
        <div className={styles.branches}>
          {branches.map((b) => (
            <PipelineFlow key={b.branch} model={model} counts={b.counts} branch={b.branch} />
          ))}
        </div>
      ) : (
        <PipelineFlow model={model} counts={counts} />
      )}
      <OverviewKpis model={model} />
      <RuntimeCard routeId={route.id} />
      {error && (
        <EmptyState
          compact
          title={OverviewText.unavailableTitle}
          description={OverviewText.unavailableBody}
          action={<Button size="sm" onClick={reload}>{OverviewText.retry}</Button>}
        />
      )}
      {!error && empty && <EmptyState compact title={OverviewText.emptyTitle} description={OverviewText.emptyBody} />}
      {!error && !empty && charts && <OverviewCharts data={charts} />}
      {!error && !charts && <EmptyState compact title={OverviewText.loading} />}
    </div>
  );
}
