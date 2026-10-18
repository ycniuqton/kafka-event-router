import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule } from '../core/HttpServiceModule';
import { HttpApprovalsService } from './HttpApprovalsService';

/** Service key of approvals. */
const ApprovalsKey = 'approvals' as const satisfies keyof Services;

/** REST module for `services.approvals` (auto-discovered by HttpServicesFactory). */
export class HttpApprovalsModule extends HttpServiceModule<typeof ApprovalsKey> {
  readonly key = ApprovalsKey;

  create(context: HttpContext): Services[typeof ApprovalsKey] {
    return new HttpApprovalsService(context);
  }
}

export default new HttpApprovalsModule();
