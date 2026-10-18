import { Format } from '../../../components/ui';
import { Tone } from '../../../domain';
import { MetricsUnavailable, type HealthSummary } from '../model/DashboardSummary';
import { KpiCard } from './KpiCard';

/** Total consumer lag, green / amber / red against the server thresholds; "Unavailable" without metrics. */
export function LagHealthCard({ summary }: { summary: HealthSummary }) {
  if (!summary.metricsAvailable) {
    return <KpiCard label="Total consumer lag" value={MetricsUnavailable.value} valueTone={Tone.Muted} footer={MetricsUnavailable.footer} />;
  }
  return <KpiCard label="Total consumer lag" value={Format.number(summary.totalLag)} valueTone={summary.lagTone} footer={summary.lagNote} />;
}
