import type { CSSProperties } from 'react';
import { ChartScale, type ChartSeries } from './ChartScale';
import styles from './LineChart.module.css';

type LineChartProps = { series: ChartSeries[]; unit: string; height?: number; tooltip: (index: number) => string; ariaLabel: string };

/** SVG line chart with a recessive 0 / half / max grid and per-point hover titles. */
export function LineChart({ series, unit, height = 120, tooltip, ariaLabel }: LineChartProps) {
  const max = ChartScale.niceMax(series.flatMap((s) => s.values));
  const count = series[0]?.values.length ?? 0;
  const width = Math.max(1, count - 1) * ChartScale.step;
  const ticks = [max, max / 2, 0];
  return (
    <div className={styles.chart} style={{ '--chart-h': `${height}px` } as CSSProperties}>
      <div className={styles.axis}>
        {ticks.map((t) => (
          <span key={t}>{`${t} ${unit}`}</span>
        ))}
      </div>
      <svg className={styles.plot} viewBox={`0 0 ${width} ${ChartScale.height}`} preserveAspectRatio="none" role="img" aria-label={ariaLabel}>
        {ticks.map((t) => (
          <line key={t} className={styles.grid} x1={0} x2={width} y1={ChartScale.y(t, max)} y2={ChartScale.y(t, max)} />
        ))}
        {series.map((s) => (
          <polyline key={s.key} data-tone={s.tone} className={styles.line} points={ChartScale.points(s.values, max)} />
        ))}
        {Array.from({ length: count }, (_, i) => (
          <rect key={i} className={styles.hit} x={(i - 0.5) * ChartScale.step} y={0} width={ChartScale.step} height={ChartScale.height}>
            <title>{tooltip(i)}</title>
          </rect>
        ))}
      </svg>
    </div>
  );
}
