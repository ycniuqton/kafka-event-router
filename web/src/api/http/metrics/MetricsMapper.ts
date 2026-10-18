import type { DashboardStats } from '../../../domain';
import type { DashboardSummaryDto } from './MetricsDtos';

/** Maps the server summary onto the dashboard stats the screens read. */
export class MetricsMapper {
  static stats(dto: DashboardSummaryDto): DashboardStats {
    const total = dto.eventsLastHour?.total ?? 0;
    return {
      eventsLastHour: Math.round(total),
      sparkSeed: Math.round(total),
      spark: dto.eventsLastHour?.spark ?? [],
      totalLag: dto.totalLag,
      deadLetters24h: dto.dlq24h?.value ?? null,
      routes: dto.routes,
      thresholds: dto.thresholds,
      metricsAvailable: dto.metricsAvailable,
    };
  }
}
