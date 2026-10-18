import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpMetricsService } from './HttpMetricsService';

/** REST module for `services.metrics` (auto-discovered by HttpServicesFactory). */
export class HttpMetricsModule extends HttpServiceModule<typeof ServiceKeys.Metrics> {
  readonly key = ServiceKeys.Metrics;

  create(context: HttpContext): Services[typeof ServiceKeys.Metrics] {
    return new HttpMetricsService(context);
  }
}

export default new HttpMetricsModule();
