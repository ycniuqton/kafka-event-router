import { Fragment } from 'react';
import { Format } from '../../../components/ui';
import type { FlowCounts, RouteDetailModel } from '../model/RouteDetailModel';
import styles from './PipelineFlow.module.css';

type PipelineFlowProps = { model: RouteDetailModel; counts: FlowCounts | null; branch?: string };

/** Horizontal source → filter → transform → destination flow with counts on each arrow (one per branch when set). */
export function PipelineFlow({ model, counts, branch }: PipelineFlowProps) {
  const n = (value: number | undefined) => (value === undefined ? Format.placeholder : Format.number(value));
  const nodes = [
    { label: 'SOURCE', text: model.sourceText, arrow: `${n(counts?.read)} read` },
    { label: 'FILTER', text: model.route.filterSummary, arrow: `${n(counts?.passed)} passed` },
    { label: 'TRANSFORM', text: model.transformLabel, arrow: `${n(counts?.sent)} sent`, note: `${n(counts?.deadLetters)} to dead letters` },
    { label: branch ? 'BRANCH → DESTINATION' : 'DESTINATION', text: branch ?? model.destinationText },
  ];
  return (
    <div className={styles.flow} aria-label={branch ? `Pipeline branch ${branch}` : 'Pipeline'}>
      {nodes.map((n) => (
        <Fragment key={n.label}>
          <div className={styles.node}>
            <div className={styles.label}>{n.label}</div>
            {n.text}
          </div>
          {n.arrow && (
            <div className={styles.arrow}>
              <div>{n.arrow}</div>
              <div className={styles.line} />
              {n.note && <div className={styles.note}>{n.note}</div>}
            </div>
          )}
        </Fragment>
      ))}
    </div>
  );
}
