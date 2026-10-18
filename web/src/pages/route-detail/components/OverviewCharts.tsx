import { Format } from '../../../components/ui';
import { Tone } from '../../../domain';
import { ChartCard } from '../charts/ChartCard';
import { ChartScale, type ChartSeries } from '../charts/ChartScale';
import { ColumnChart } from '../charts/ColumnChart';
import { LineChart } from '../charts/LineChart';
import type { OverviewChartData } from '../model/OverviewSeriesView';
import styles from './OverviewCharts.module.css';

/** Chart axis constants: percentage scale of the relative throughput bars, units of the line charts. */
const ChartUnits = { percentMax: 100, ms: 'ms', perMin: '/min', partitionGap: 0.45, lagMaxPct: 100 } as const;

/** The four overview charts: in vs out, lag per partition, latency p50/p95, errors per minute. */
export function OverviewCharts({ data }: { data: OverviewChartData }) {
  const { io, lag, latency, errors } = data;
  const ioSeries: ChartSeries[] = [
    { key: 'in', label: 'in', values: io.map((p) => p.inPct) },
    { key: 'out', label: 'out', values: io.map((p) => p.outPct), tone: Tone.Accent },
  ];
  const latencySeries: ChartSeries[] = [
    { key: 'p50', label: 'p50', values: latency.map((p) => p.p50), tone: Tone.Accent },
    { key: 'p95', label: 'p95', values: latency.map((p) => p.p95), tone: Tone.Code },
  ];
  const errorSeries: ChartSeries[] = [{ key: 'errors', label: 'errors', values: errors, tone: Tone.Err }];
  return (
    <div className={styles.grid}>
      <ChartCard title="Events/sec · in vs out" legend={ioSeries}>
        <ColumnChart series={ioSeries} max={ChartUnits.percentMax} ariaLabel="Events per second, in vs out" tooltip={(i) => `in ${io[i].inPct}% · out ${io[i].outPct}% of peak`} />
      </ChartCard>
      <ChartCard title="Consumer lag per partition">
        <ColumnChart
          series={[{ key: 'lag', label: 'lag', values: lag.map((p) => p.heightPct), tone: Tone.Warn }]}
          max={ChartUnits.lagMaxPct}
          gapRatio={ChartUnits.partitionGap}
          topLabels={lag.map((p) => p.label)}
          bottomLabels={lag.map((p) => p.partition)}
          ariaLabel="Consumer lag per partition"
          tooltip={(i) => `${lag[i].partition} · ${Format.number(lag[i].lag)} behind`}
        />
      </ChartCard>
      <ChartCard title="Latency · p50 / p95" legend={latencySeries}>
        <LineChart series={latencySeries} unit={ChartUnits.ms} ariaLabel="End-to-end latency" tooltip={(i) => `p50 ${latency[i].p50} ms · p95 ${latency[i].p95} ms`} />
      </ChartCard>
      <ChartCard title="Errors / min">
        <ColumnChart series={errorSeries} max={ChartScale.niceMax(errors)} ariaLabel="Errors per minute" tooltip={(i) => `${errors[i]}${ChartUnits.perMin}`} />
      </ChartCard>
    </div>
  );
}
