import type { Services } from '../../contracts';
import type { HttpContext } from '../core/HttpContext';
import { HttpServiceModule, ServiceKeys } from '../core/HttpServiceModule';
import { HttpSandboxService } from './HttpSandboxService';

/** REST module for `services.sandbox` (auto-discovered by HttpServicesFactory). */
export class HttpSandboxModule extends HttpServiceModule<typeof ServiceKeys.Sandbox> {
  readonly key = ServiceKeys.Sandbox;

  create(context: HttpContext): Services[typeof ServiceKeys.Sandbox] {
    return new HttpSandboxService(context);
  }
}

export default new HttpSandboxModule();
