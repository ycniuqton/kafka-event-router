import type { CSSProperties } from 'react';
import { ChartScale, type ChartSeries } from './ChartScale';
import styles from './ColumnChart.module.css';

type ColumnChartProps = {
  series: ChartSeries[];
  max: number;
  /** Share of each slot left empty between columns (0–1). */
  gapRatio?: number;
  height?: number;
  topLabels?: string[];
  bottomLabels?: string[];
  tooltip: (index: number) => string;
  ariaLabel: string;
};

/** SVG column chart; several series are overlaid front-to-back in order (e.g. in behind out). */
export function ColumnChart({ series, max, gapRatio = 0.28, height = 120, topLabels, bottomLabels, tooltip, ariaLabel }: ColumnChartProps) {
  const count = series[0]?.values.length ?? 0;
  const width = count * ChartScale.step;
  const barWidth = ChartScale.step * (1 - gapRatio);
  const inset = (ChartScale.step - barWidth) / 2;
  return (
    <div className={styles.chart} style={{ '--chart-h': `${height}px` } as CSSProperties}>
      {topLabels && <AxisLabels labels={topLabels} />}
      <svg className={styles.plot} viewBox={`0 0 ${width} ${ChartScale.height}`} preserveAspectRatio="none" role="img" aria-label={ariaLabel}>
        {series.map((s) => (
          <g key={s.key} data-tone={s.tone} className={s.tone ? styles.toned : styles.neutral}>
            {s.values.map((v, i) => (
              <rect key={i} x={i * ChartScale.step + inset} y={ChartScale.y(v, max)} width={barWidth} height={ChartScale.height - ChartScale.y(v, max)} />
            ))}
          </g>
        ))}
        {Array.from({ length: count }, (_, i) => (
          <rect key={i} className={styles.hit} x={i * ChartScale.step} y={0} width={ChartScale.step} height={ChartScale.height}>
            <title>{tooltip(i)}</title>
          </rect>
        ))}
      </svg>
      {bottomLabels && <AxisLabels labels={bottomLabels} muted />}
    </div>
  );
}

/** Evenly spaced labels aligned with the columns. */
function AxisLabels({ labels, muted }: { labels: string[]; muted?: boolean }) {
  return (
    <div className={muted ? `${styles.labels} ${styles.muted}` : styles.labels}>
      {labels.map((l, i) => (
        <span key={i}>{l}</span>
      ))}
    </div>
  );
}
