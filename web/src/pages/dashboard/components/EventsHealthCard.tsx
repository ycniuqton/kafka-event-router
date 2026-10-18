import { Format, Sparkline } from '../../../components/ui';
import { Tone, type DashboardStats } from '../../../domain';
import { MetricsUnavailable } from '../model/DashboardSummary';
import { KpiCard } from './KpiCard';

/** Dashboard sparkline size (design: 110×30, 18 bars). */
const EventsSpark = { width: 110, height: 30 } as const;

/** Events forwarded in the last hour with the server's 5-minute sparkline; degrades to "Unavailable". */
export function EventsHealthCard({ stats }: { stats: DashboardStats | undefined }) {
  if (stats?.metricsAvailable === false) {
    return <KpiCard label="Events forwarded · last hour" value={MetricsUnavailable.value} valueTone={Tone.Muted} footer={MetricsUnavailable.footer} />;
  }
  const value = stats ? Format.number(stats.eventsLastHour) : Format.placeholder;
  const aside = stats && <Sparkline seed={stats.sparkSeed} values={stats.spark} width={EventsSpark.width} height={EventsSpark.height} wide />;
  return <KpiCard label="Events forwarded · last hour" value={value} aside={aside} />;
}
