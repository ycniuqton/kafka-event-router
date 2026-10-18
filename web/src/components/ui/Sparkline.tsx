import type { CSSProperties } from 'react';
import { Tone } from '../../domain';
import { SparkSeries } from './SparkSeries';
import styles from './Sparkline.module.css';

export type SparklineProps = {
  seed: number;
  /** Real series to draw instead of the seeded placeholder shape. */
  values?: number[];
  /** Render the flat "idle" series (paused / zero-throughput routes). */
  flat?: boolean;
  tone?: Tone;
  width?: number;
  height?: number;
  /** Wider spacing used by the dashboard KPI card. */
  wide?: boolean;
};

/** Bar sparkline: 54×18 in route rows, 110×30 on the dashboard. */
export function Sparkline({ seed, values, flat, tone = Tone.Accent, width = 54, height = 18, wide }: SparklineProps) {
  const size = { '--spark-w': `${width}px`, '--spark-h': `${height}px` } as CSSProperties;
  return (
    <div className={[styles.spark, wide && styles.wide].filter(Boolean).join(' ')} style={size} data-tone={tone} aria-hidden>
      {(values?.length ? SparkSeries.fromValues(values) : SparkSeries.heights(seed, flat)).map((h, i) => (
        <div key={i} className={styles.bar} style={{ '--h': `${h}%` } as CSSProperties} />
      ))}
    </div>
  );
}
