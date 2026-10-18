import { Format } from '../../../components/ui';
import type { RouteDetailModel } from '../model/RouteDetailModel';
import { KpiTile } from './KpiTile';
import styles from './OverviewKpis.module.css';

/** Events/sec in → out, consumer lag, latency p50 / p95 and errors/min. */
export function OverviewKpis({ model }: { model: RouteDetailModel }) {
  const { p50, p95 } = model.latency;
  return (
    <div className={styles.grid}>
      <KpiTile label="Events/sec in → out">
        {Format.number(model.eventsIn)} → {Format.number(model.eventsOut)}
      </KpiTile>
      <KpiTile label="Consumer lag" tone={model.lagTone}>
        {model.lagText}
      </KpiTile>
      <KpiTile label="Latency p50 / p95">
        {p50} ms / {p95} ms
      </KpiTile>
      <KpiTile label="Errors / min" tone={model.errorTone}>
        {model.errorsPerMin}
      </KpiTile>
    </div>
  );
}
