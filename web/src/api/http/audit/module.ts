import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpAuditService } from './HttpAuditService';

/** REST module for `services.audit` (auto-discovered by HttpServicesFactory). */
export class HttpAuditModule extends HttpServiceModule<typeof ServiceKeys.Audit> {
  readonly key = ServiceKeys.Audit;

  create(context: HttpContext): Services[typeof ServiceKeys.Audit] {
    return new HttpAuditService(context);
  }
}

export default new HttpAuditModule();
