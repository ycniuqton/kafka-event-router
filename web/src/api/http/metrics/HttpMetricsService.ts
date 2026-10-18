import { LagHealth, type DashboardStats, type FlowMap, type FlowMapQuery, type MetricsRange, type RouteOverviewMetrics } from '../../../domain';
import { MetricsService } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { MetricsEndpoint, MetricsParam, type DashboardSummaryDto, type RouteMetricsDto } from './MetricsDtos';
import { MetricsMapper } from './MetricsMapper';

/**
 * `services.metrics` over `/api/v1`: dashboard summary (adopts server lag thresholds) and route overview metrics.
 * Overview failures (503 metrics unavailable) are silent: the Overview tab shows its own retry state.
 */
export class HttpMetricsService extends MetricsService {
  private readonly context: HttpContext;

  constructor(context: HttpContext) {
    super();
    this.context = context;
  }

  async dashboard(): Promise<DashboardStats> {
    const dto = await this.context.api.get<DashboardSummaryDto>(MetricsEndpoint.summary);
    LagHealth.configure(dto.thresholds);
    return MetricsMapper.stats(dto);
  }

  routeMetrics(routeId: string, range: MetricsRange): Promise<RouteOverviewMetrics> {
    const query = { [MetricsParam.range]: range };
    return this.context.api.get<RouteMetricsDto>(MetricsEndpoint.routeMetrics(routeId), query, { silent: true });
  }

  /** `GET /flow-map`; empty filters are not sent. */
  flowMap(query: FlowMapQuery): Promise<FlowMap> {
    const params = Object.fromEntries(Object.entries(query).filter(([, v]) => v !== ''));
    return this.context.api.get<FlowMap>(MetricsEndpoint.flowMap, params);
  }
}
