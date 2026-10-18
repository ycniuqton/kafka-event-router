import { RouteStatus, RouteStatusMeta } from '../../../domain';
import { FlowEdgeStyle } from '../model/FlowGraph';
import styles from './FlowLegend.module.css';

const LegendStatuses = [RouteStatus.Running, RouteStatus.Error, RouteStatus.Paused] as const;

/** "━ running ━ error ━ paused" key, coloured like the lines. */
export function FlowLegend() {
  return (
    <div className={styles.legend}>
      {LegendStatuses.map((s) => (
        <span key={s} className={styles.item} data-tone={s === RouteStatus.Paused ? undefined : FlowEdgeStyle.tone(s)}>
          ━ {RouteStatusMeta[s].label.toLowerCase()}
        </span>
      ))}
    </div>
  );
}
