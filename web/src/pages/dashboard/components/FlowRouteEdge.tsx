import { getBezierPath, type EdgeProps } from '@xyflow/react';
import type { CSSProperties } from 'react';
import { RouteStatus } from '../../../domain';
import { FlowEdgeStyle, type RouteEdge } from '../model/FlowGraph';
import styles from './FlowRouteEdge.module.css';

/** A route line: faint base, animated dashes when running, dotted overlay in error, wide invisible hit area. */
export function FlowRouteEdge({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition, data }: EdgeProps<RouteEdge>) {
  const [path] = getBezierPath({ sourceX, sourceY, targetX, targetY, sourcePosition, targetPosition });
  if (!data) return null;
  const vars = {
    '--edge-w': `${data.width}px`,
    '--edge-hover-w': `${data.width + FlowEdgeStyle.hoverBoost}px`,
    '--flow-s': `${data.flowSeconds}s`,
  } as CSSProperties;
  return (
    <g className={styles.edge} data-tone={data.tone} data-hovered={data.hovered} style={vars}>
      <path className={styles.base} d={path} />
      {data.status === RouteStatus.Running && <path className={styles.flow} d={path} />}
      {data.status === RouteStatus.Error && <path className={styles.error} d={path} />}
      <path className={styles.hit} d={path} />
    </g>
  );
}
