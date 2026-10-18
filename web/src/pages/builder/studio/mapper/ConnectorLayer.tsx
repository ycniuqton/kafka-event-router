import type { RefObject } from 'react';
import type { ConnectorLink, ConnectorRegistry } from './ConnectorRegistry';
import styles from './VisualMapper.module.css';

type ConnectorLayerProps = { registry: ConnectorRegistry; container: RefObject<HTMLElement | null>; link: ConnectorLink | null };

/** SVG overlay drawing the hovered mapping as a curve from its input field to its output field. */
export function ConnectorLayer({ registry, container, link }: ConnectorLayerProps) {
  const el = container.current;
  const line = link && el ? registry.line(link, el) : null;
  if (!line) return null;
  const mid = (line.x1 + line.x2) / 2;
  return (
    <svg className={styles.connectors} aria-hidden="true">
      <path className={styles.connector} d={`M ${line.x1} ${line.y1} C ${mid} ${line.y1}, ${mid} ${line.y2}, ${line.x2} ${line.y2}`} />
      <circle className={styles.connectorEnd} cx={line.x1} cy={line.y1} r={3} />
      <circle className={styles.connectorEnd} cx={line.x2} cy={line.y2} r={3} />
    </svg>
  );
}
